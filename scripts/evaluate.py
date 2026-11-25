"""Run both labeled datasets through real HTTP, queues and stores with normal quotas."""

import hashlib
import json
import math
import os
import subprocess
import tempfile
import uuid
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "evaluation"


def ratio(n, d):
    return round(n / d, 4) if d else None


def percentile(values, p):
    return round(sorted(values)[math.ceil(len(values) * p) - 1], 2)


def report(invoices, questions, environment):
    assert len(invoices) == 20 and len(questions) == 30
    text = [r for r in invoices if r["group"] == "text"]
    fields = [
        (expected, r["fields"].get(k))
        for r in text
        for k, expected in r["expected_fields"].items()
    ]
    present = [(e, a) for e, a in fields if e is not None]
    absent = [(e, a) for e, a in fields if e is None]
    rules = defaultdict(lambda: [0, 0, 0])
    for r in invoices:
        gold, actual = set(r["expected_rules"]), set(r["rules"])
        for k in gold | actual:
            rules[k][0] += k in gold & actual
            rules[k][1] += k in actual - gold
            rules[k][2] += k in gold - actual
    tp, fp, fn = map(sum, zip(*rules.values()))
    answerable = [q for q in questions if q["clause"]]
    unsupported = [q for q in questions if not q["clause"]]
    answered = [q for q in questions if not q["abstained"]]
    metrics = {
        "invoice_cases": len(invoices),
        "policy_questions": len(questions),
        "text_invoices_scored": len(text),
        "unsupported_document_cases": len(invoices) - len(text),
        "nonmissing_fields_correct": sum(e == a for e, a in present),
        "nonmissing_fields_scored": len(present),
        "field_accuracy": ratio(sum(e == a for e, a in present), len(present)),
        "missing_fields_correct": sum(e == a for e, a in absent),
        "missing_fields_scored": len(absent),
        "invoice_routing_accuracy": ratio(
            sum(r["status"] == r["expected_status"] for r in invoices), len(invoices)
        ),
        "validation_precision": ratio(tp, tp + fp),
        "validation_recall": ratio(tp, tp + fn),
        "answer_rate_on_answerable": ratio(
            sum(not q["abstained"] for q in answerable), len(answerable)
        ),
        "expected_clause_on_answerable": ratio(
            sum(
                any(c["clause"] == q["clause"] for c in q["citations"])
                for q in answerable
            ),
            len(answerable),
        ),
        "citation_quote_validity": ratio(
            sum(q["quoted"] for q in answered), len(answered)
        ),
        "abstention_recall": ratio(
            sum(q["abstained"] for q in unsupported), len(unsupported)
        ),
        "human_reviewed_answer_support": None,
        "invoice_job_failure_rate": ratio(
            sum(r["job_status"] != "SUCCEEDED" for r in invoices), len(invoices)
        ),
        "estimated_cost_aed": None,
    }
    for name, values in [
        ("invoice_end_to_end", [r["elapsed_ms"] for r in invoices]),
        ("invoice_enqueue_http", [r["gateway_ms"] for r in invoices]),
        ("policy_http", [q["elapsed_ms"] for q in questions]),
    ]:
        for p in (50, 95):
            metrics[f"{name}_p{p}_ms"] = percentile(values, p / 100)
    digest = hashlib.sha256()
    for path in sorted((ROOT / "fixtures/evaluation").rglob("*")):
        if path.is_file():
            digest.update(path.name.encode())
            digest.update(path.read_bytes())
    result = {
        "status": "measured",
        "date": datetime.now(timezone.utc).isoformat(),
        "dataset": "fictional-v1",
        "dataset_sha256": digest.hexdigest(),
        "mode": "mock",
        "model": None,
        "environment": environment,
        "method": "Two clients run concurrently, one sequential invoice client and one sequential policy client. Each starts a fresh workspace every ten requests. Worker concurrency is two. End-to-end invoice latency includes polling at 50 ms. Percentiles use nearest rank.",
        "limitations": "Synthetic English labels share the fixture layout. Scan and Arabic cases are evaluated only for routing. Quote and expected-clause checks are automated. Human support ratings, real model quality, token usage and cost are unavailable.",
        "metrics": metrics,
        "by_rule": {
            k: {
                "tp": t,
                "fp": f,
                "fn": n,
                "precision": ratio(t, t + f),
                "recall": ratio(t, t + n),
            }
            for k, (t, f, n) in sorted(rules.items())
        },
        "invoices": invoices,
        "questions": questions,
    }
    OUT.mkdir(exist_ok=True)
    table = "\n".join(
        f"| {k.replace('_', ' ')} | {v if v is not None else 'Unavailable'} |"
        for k, v in metrics.items()
    )
    markdown = f"""# NexusAI mock evaluation

Measured {result["date"]} in {environment}. Dataset fictional-v1 includes 20 invoice PDFs and 30 policy questions (20 answerable and 10 unsupported).

{result["method"]}

| Measure | Observed |
|---|---:|
{table}

{result["limitations"]}

Per-case outputs, citations, expected labels, per-rule confusion counts and dataset hash are in [report.json](report.json). Ratios use a 0–1 scale. A missing denominator is reported as unavailable.
"""
    for name, content in [
        ("report.json", json.dumps(result, indent=2) + "\n"),
        ("REPORT.md", markdown),
    ]:
        tmp = OUT / (name + ".tmp")
        tmp.write_text(content)
        tmp.replace(OUT / name)
    print(json.dumps(metrics, indent=2))


def run():
    project = "nexusai-eval-" + uuid.uuid4().hex[:12]
    args = [
        "docker",
        "compose",
        "-p",
        project,
        "-f",
        "compose.yaml",
        "-f",
        "infra/eval.compose.yaml",
    ]
    (ROOT / ".cache").mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(dir=ROOT / ".cache") as tmp:
        env = {
            **os.environ,
            "EVAL_OUTPUT": tmp,
            "EVAL_UID": str(os.getuid()),
            "EVAL_GID": str(os.getgid()),
        }

        def compose(*cmd, **kwargs):
            return subprocess.run(
                args + list(cmd), cwd=ROOT, env=env, check=True, **kwargs
            )

        try:
            compose("up", "-d", "--build", "--wait", "gateway", "ai-api", "ai-worker")
            compose("up", "-d", "--build", "invoice-bench", "policy-bench")
            compose("wait", "invoice-bench", "policy-bench")
            records = [
                json.loads((Path(tmp) / (name + ".json")).read_text())
                for name in ("invoices", "questions")
            ]
            report(records[0]["rows"], records[1]["rows"], records[0]["environment"])
        except Exception:
            compose("logs", "--tail", "20", "migrate", "seed", "gateway", "ai-api", "invoice-bench", "policy-bench")
            raise
        finally:
            # The random project name confines cleanup to this invocation's synthetic test data.
            compose("down", "--volumes")


if __name__ == "__main__":
    run()
