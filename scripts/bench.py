"""Benchmark clients for the disposable, unexposed local evaluation stack."""

import json
import os
import platform
import sys
import time
import uuid
from pathlib import Path

import psycopg

from smoke import Client

ROOT = Path("/dataset")
V1 = "/api/v1"


def invoices():
    cases = json.loads((ROOT / "invoices.json").read_text())
    with psycopg.connect(os.environ["TEST_DB"]) as db:
        if db.execute("SELECT current_database()").fetchone() != ("nexus_eval",):
            raise RuntimeError("Evaluation requires its dedicated database")
        for case in cases:
            sid = case["sample_id"]
            db.execute(
                "INSERT INTO samples VALUES (%s,%s,'Fictional evaluation case')",
                (sid, sid),
            )
    results = []
    for index, case in enumerate(cases):
        if index % 10 == 0:
            client = Client()
            client.start()
        start = time.perf_counter()
        request = client.request(
            "POST",
            V1 + "/invoices",
            202,
            {"sample_id": case["sample_id"]},
            **{"Idempotency-Key": str(uuid.uuid4())},
        )
        gateway = (time.perf_counter() - start) * 1000
        while time.perf_counter() - start < 125:
            job = client.request("GET", V1 + "/jobs/" + request["job_id"])
            if job["status"] in {"FAILED", "SUCCEEDED"}:
                break
            time.sleep(0.05)
        detail = client.request("GET", V1 + "/invoices/" + request["invoice_id"])
        elapsed = (time.perf_counter() - start) * 1000
        rev = detail["revision"] or {}
        results.append(
            {
                "sample_id": case["sample_id"],
                "group": case["group"],
                "status": detail["invoice"]["status"],
                "expected_status": case["state"],
                "fields": rev.get("fields", {}),
                "expected_fields": case["fields"],
                "rules": sorted(r["rule_code"] for r in detail["validation_results"]),
                "expected_rules": sorted(case["rules"]),
                "job_status": job["status"],
                "gateway_ms": round(gateway, 2),
                "elapsed_ms": round(elapsed, 2),
            }
        )
    return results


def questions():
    results = []
    for i, case in enumerate(json.loads((ROOT / "questions.json").read_text())):
        if i % 10 == 0:
            client = Client()
            client.start()
        start = time.perf_counter()
        result = client.request(
            "POST", V1 + "/policy/questions", 200, {"question": case["question"]}
        )
        sources = result["citations"]
        results.append(
            {
                **case,
                "answer": result["answer"],
                "abstained": result["abstained"],
                "citations": sources,
                "quoted": bool(sources)
                and all(s["excerpt"] == result["answer"] for s in sources),
                "elapsed_ms": round((time.perf_counter() - start) * 1000, 2),
            }
        )
    # Refreshing the workspace after thirty questions on this client IP must still hit its hourly cap.
    another = Client()
    another.start()
    another.request(
        "POST", V1 + "/policy/questions", 429, {"question": "Who approves purchases?"}
    )
    return results


if __name__ == "__main__":
    suite = sys.argv[1]
    rows = {"invoices": invoices, "questions": questions}[suite]()
    (Path("/results") / (suite + ".json")).write_text(
        json.dumps(
            {
                "environment": f"Linux {platform.machine()}, Python {platform.python_version()}, Docker Compose, mock provider",
                "rows": rows,
            },
            indent=2,
        )
    )
    print(f"Measured {len(rows)} {suite}")
