import asyncio
import logging
import os
import socket
import time

import httpx
from redis.asyncio import Redis
from redis.exceptions import RedisError, ResponseError

from app.extract import DocumentError, extract, fixture

STREAM = "invoice.jobs"
GROUP = "extract-v1"
log = logging.getLogger("worker")


async def process(http: httpx.AsyncClient, job_id: str) -> bool:
    base = f"/internal/jobs/{job_id}"
    response = await http.post(base + "/claim", json={})
    if response.status_code == 404:
        return True  # Workspace expired and purged.
    response.raise_for_status()
    claim = response.json()
    if claim["disposition"] in {"busy", "terminal"}:
        return True
    attempt = {"attempt": claim["attempt"], "lease_token": claim["lease_token"]}
    started = time.monotonic()
    try:
        result = await asyncio.wait_for(
            asyncio.to_thread(extract, fixture(claim["sample_id"])), 60
        )
        action = "complete"
        payload = {"extraction": result.model_dump()}
    except (DocumentError, TimeoutError, RuntimeError, OSError) as error:
        code = str(error) if isinstance(error, DocumentError) else "PROVIDER_ERROR"
        action = "fail"
        payload = {"error_code": code, "retryable": code == "PROVIDER_ERROR"}
    latency = round((time.monotonic() - started) * 1000)
    reply = await http.post(
        base + "/" + action, json={**attempt, **payload, "latency_ms": latency}
    )
    if reply.status_code in {404, 409}:
        return True
    reply.raise_for_status()
    return reply.json()["disposition"] == "accepted"


async def consume(redis: Redis, http: httpx.AsyncClient, name: str):
    cursor = "0-0"
    while True:
        try:
            try:
                await redis.xgroup_create(STREAM, GROUP, "0", mkstream=True)
            except ResponseError as error:
                if "BUSYGROUP" not in str(error):
                    raise
            claimed = await redis.xautoclaim(
                STREAM, GROUP, name, 120_000, cursor, count=1
            )
            cursor, entries = claimed[:2]
            if not entries:
                batches = await redis.xreadgroup(
                    GROUP, name, {STREAM: ">"}, count=1, block=1500
                )
                entries = batches[0][1] if batches else []
            for message_id, fields in entries:
                if await process(http, fields["job_id"]):
                    await redis.xack(STREAM, GROUP, message_id)
        except (RedisError, httpx.HTTPError, KeyError):
            log.warning("Job delivery deferred")
            await asyncio.sleep(2)


async def main():
    async with Redis.from_url(os.environ["REDIS_URL"], decode_responses=True) as redis:
        async with httpx.AsyncClient(
            base_url="http://gateway:8081",
            timeout=10,
            headers={"Authorization": "Bearer " + os.environ["SERVICE_TOKEN"]},
        ) as http:
            await asyncio.gather(
                *(consume(redis, http, f"{socket.gethostname()}-{i}") for i in range(2))
            )


if __name__ == "__main__":
    asyncio.run(main())
