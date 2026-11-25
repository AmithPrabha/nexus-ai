"""Live public workflow and private lease recovery against disposable demo records."""

import os
import time
import uuid

import httpx
import psycopg
from redis import Redis
from smoke import Client

V1 = "/api/v1"


def until_ready(client, queued):
    for _ in range(100):
        j = client.request("GET", f"{V1}/jobs/{queued['job_id']}")
        if j["status"] == "SUCCEEDED":
            return client.request("GET", f"{V1}/invoices/{queued['invoice_id']}")
        assert j["status"] != "FAILED", j
        time.sleep(0.2)
    raise AssertionError("Worker did not finish within 20s")


def create(c, sample, key=None):
    return c.request(
        "POST",
        V1 + "/invoices",
        202,
        {"sample_id": sample},
        **{"Idempotency-Key": key or str(uuid.uuid4())},
    )


def public_checks():
    c, other = Client(), Client()
    work = c.start()
    other.start()
    cases = {
        "office-supplies": None,
        "design-services": "VAT_MISMATCH",
        "equipment": "APPROVAL_THRESHOLD",
        "courier": "MISSING_SUPPLIER_TRN",
        "maintenance": "TRN_FORMAT",
    }
    c.request(
        "POST",
        V1 + "/invoices",
        400,
        {"sample_id": "unknown"},
        **{"Idempotency-Key": "invalid"},
    )
    c.request(
        "POST", V1 + "/invoices", 403, {"sample_id": "courier"}, **{"X-CSRF-Token": ""}
    )
    for sample, rule in cases.items():
        queued = create(c, sample, sample)
        assert create(c, sample, sample) == queued
        c.request(
            "POST",
            V1 + "/invoices",
            409,
            {"sample_id": "unknown"},
            **{"Idempotency-Key": sample},
        )
        state = until_ready(c, queued)
        assert state["invoice"]["status"] == (
            "PENDING_REVIEW" if rule else "READY_FOR_APPROVAL"
        )
        codes = [item["rule_code"] for item in state["validation_results"]]
        assert codes == ([rule] if rule else []), codes
        path = V1 + "/invoices/" + queued["invoice_id"]
        other.request("GET", path, 404)
        other.request("GET", path + "/audit", 404)
        other.request("GET", V1 + "/jobs/" + queued["job_id"], 404)
        version = state["version"]
        if sample == "design-services":
            c.request(
                "POST",
                path + "/decision",
                400,
                {"version": version, "decision": "approve", "reason": ""},
            )
            edit = {
                "fields": {"vat_amount": "24.00", "total": "504.00"},
                "reason": "Checked the fictional invoice",
            }
            c.request("PATCH", path + "/review", 409, {**edit, "version": version - 1})
            state = c.request(
                "PATCH", path + "/review", 200, {**edit, "version": version}
            )
            assert (
                state["invoice"]["status"] == "READY_FOR_APPROVAL"
                and not state["validation_results"]
            )
            version = state["version"]
        choice = "reject" if sample == "maintenance" else "approve"
        body = {
            "version": version,
            "decision": choice,
            "reason": "Simulated human review",
        }
        other.request("POST", path + "/decision", 404, body)
        final = c.request("POST", path + "/decision", 200, body)
        assert final["status"] == ("APPROVED" if choice == "approve" else "REJECTED")
        c.request(
            "POST", path + "/decision", 409, {**body, "version": final["version"]}
        )
        history = c.request("GET", path + "/audit")["items"]
        assert (
            history[-1]["actor"] == "demo_reviewer" and history[-1]["action"] == choice
        )
        assert set(c.request("GET", path + "/audit?limit=1")["items"][0]) == {
            "id",
            "actor",
            "action",
            "from_state",
            "to_state",
            "reason",
            "request_id",
            "created_at",
        }
        assert c.request("GET", path + "/audit?limit=1")["next_cursor"]
    dup = until_ready(c, create(c, "office-supplies"))
    assert "DUPLICATE_INVOICE" in [r["rule_code"] for r in dup["validation_results"]]
    assert other.request("GET", V1 + "/invoices")["items"] == []
    print(
        "Five invoice workflows, corrections, decisions, replay, duplicates and session isolation passed",
        flush=True,
    )
    return work, c


def recovery_checks(work, client):
    # Create dedicated queued rows without an outbox so the production worker cannot race the lease test.
    with (
        psycopg.connect(os.environ["TEST_DB"], autocommit=True) as db,
        httpx.Client(
            base_url="http://gateway:8081",
            headers={"Authorization": "Bearer " + os.environ["SERVICE_TOKEN"]},
        ) as private,
    ):

        def queued():
            (inv,) = db.execute(
                "INSERT INTO invoices(workspace_id,sample_id,status) VALUES(%s,'courier','QUEUED') RETURNING id",
                (work,),
            ).fetchone()
            (job,) = db.execute(
                "INSERT INTO jobs(invoice_id) VALUES(%s) RETURNING id", (inv,)
            ).fetchone()
            return str(inv), str(job)

        def call(job, action, body, expected=200):
            response = private.post(f"/internal/jobs/{job}/{action}", json=body)
            assert response.status_code == expected, (
                action,
                response.status_code,
                response.text,
            )
            return response.json()

        inv, job = queued()
        claim = call(job, "claim", {})
        assert call(job, "claim", {})["disposition"] == "busy"
        old = {
            "attempt": 1,
            "lease_token": claim["lease_token"],
            "latency_ms": 0,
            "retryable": True,
            "error_code": "PROVIDER_ERROR",
        }
        for number in (2, 3):
            db.execute(
                "UPDATE jobs SET lease_until=now()-interval '1 second' WHERE id=%s",
                (job,),
            )
            claim = call(job, "claim", {})
            assert claim["attempt"] == number
            call(job, "fail", old, 409)
        failure = {**old, "lease_token": claim["lease_token"], "attempt": 3}
        assert call(job, "fail", failure) == call(job, "fail", failure)
        assert call(job, "claim", {})["disposition"] == "terminal"
        state = client.request("GET", f"{V1}/invoices/{inv}")
        assert state["invoice"]["status"] == "FAILED"
        retry = client.request(
            "POST",
            f"{V1}/invoices/{inv}/retry",
            202,
            {"version": state["version"]},
            **{"Idempotency-Key": "recover"},
        )
        until_ready(client, retry)
        inv, job = queued()
        claim = call(job, "claim", {})
        from app.extract import extract, fixture

        done = {
            "attempt": 1,
            "lease_token": claim["lease_token"],
            "latency_ms": 0,
            "extraction": extract(fixture("courier")).model_dump(),
        }
        assert call(job, "complete", done) == call(job, "complete", done)
        assert call(job, "claim", {})["disposition"] == "terminal"
        assert db.execute(
            "SELECT count(*) FROM invoice_revisions WHERE invoice_id=%s", (inv,)
        ).fetchone() == (1,)
        # Model a crash after committing the result and before acknowledging its stream entry.
        # Keep adding/claiming atomic so a live consumer cannot take this test entry first.
        with Redis.from_url("redis://redis:6379/0", decode_responses=True) as stream:
            mid = stream.eval(
                """
                local id=redis.call('XADD',KEYS[1],'*','job_id',ARGV[1])
                redis.call('XREADGROUP','GROUP',ARGV[2],'interrupted-test','COUNT',100,'STREAMS',KEYS[1],'>')
                redis.call('XCLAIM',KEYS[1],ARGV[2],'interrupted-test',0,id,'IDLE',121000,'FORCE')
                return id
            """,
                1,
                "invoice.jobs",
                job,
                "extract-v1",
            )
            for _ in range(50):
                if not stream.xpending_range("invoice.jobs", "extract-v1", mid, mid, 1):
                    break
                time.sleep(0.1)
            else:
                raise AssertionError(
                    "Live worker did not reclaim and acknowledge the committed job"
                )
        assert db.execute(
            "SELECT count(*) FROM invoice_revisions WHERE invoice_id=%s", (inv,)
        ).fetchone() == (1,)
        assert not db.execute(
            "SELECT has_table_privilege('nexus_gateway','audit_events','UPDATE,DELETE')"
        ).fetchone()[0]
    print(
        "Lease fencing, retry exhaustion, durable replay, manual retry and append-only audit grants passed"
    )


if __name__ == "__main__":
    recovery_checks(*public_checks())
