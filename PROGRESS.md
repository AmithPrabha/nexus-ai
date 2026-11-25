# NexusAI development handoff

Local mock release gates G0–G5 passed on 2026-11-25. The working tree is based on main at 2bdb5e9. The changes are uncommitted. External deployment, source publication and the separate portfolio link await approval.

| Gate | Verified outcome |
|---|---|
| G0 | Inspected the clean documentation-only checkout, separate Next.js portfolio and installed toolchain. Restored the missing PROJECT_SPEC.md from the local final archive after verifying all seven companion files byte-for-byte. |
| G1 | Compose, checksummed migrations, restricted database roles, private credentials, health, session lifecycle and five protected fictional PDF previews. |
| G2 | Five live invoice workflows, deterministic financial checks, optimistic correction, human decisions, audit, idempotency, stale lease rejection, retry exhaustion, manual retry and worker recovery after a lost ACK. |
| G3 | Three versioned fictional policies, scoped vector and keyword retrieval, verified source quotations, answerable/unsupported/instruction tests, per-session and per-peer quotas. |
| G4 | Production Next.js build, six workspace views, desktop and mobile browser acceptance, measured session telemetry, local review receipts and optional isolated n8n workflow. A live n8n outage exhausted only notification retries and the invoice still accepted a human decision. |
| G5 | Clean-volume 20-invoice/30-question evaluation, public/private contract checks, dependency and secret scans, reproducible setup and a recorded browser walkthrough. |
| G6 | Approval required for hosting, spending, push, public smoke tests and portfolio publication. |

## Measured results

The 2026-11-25 07:19 UTC run processed 20 invoices with 20 correct routes. On 18 English text PDFs the mock recovered 177/177 populated fields and 3/3 labeled missing values. The other two cases checked scan and Arabic routing. Across 30 policy questions, all 20 answerable cases cited the expected clause, and all ten unsupported cases abstained. End-to-end invoice p50/p95 was 395.34/446.65 ms. Policy HTTP p50/p95 was 13.91/28.17 ms. Concurrency was one client per workload with two worker consumers. [Per-case results and methodology](evaluation/report.json)

## Verification ledger

| Commands run locally | Exit | Outcome |
|---|---:|---|
| docker compose up -d --build --wait; docker compose run --rm migrate; python3 scripts/smoke.py | 0 | Real PostgreSQL/pgvector and Redis, migration replay, readiness, session reset, isolated cookies, configured origin and all five PDFs. |
| go test -race -count=1 ./...; go vet ./...; gofmt check | 0 | Five top-level tests and 18 subtests including deterministic rounding, TRN, invalid citations, HMAC replay window and OpenAPI route parity. |
| Python pytest -q; ruff check app tests | 0 | Eight Python tests. Local PyMuPDF SWIG and Starlette TestClient emit deprecation warnings. |
| npm run lint; npm run typecheck; npm run contracts -- --check; Docker npm ci and next build | 0 | Locked production dashboard and synchronized types. |
| Live integration service running test_invoices.py and test_policy.py | 0 | Five invoice fixtures, recovery, corrections, decisions, scope, quotas, cited answers and abstention. The enhanced recovery test reclaimed an actual stale Redis pending entry. |
| PW_CHANNEL=chrome npm run e2e | 0 | Two desktop/mobile browser scenarios passed; the final browser rerun took 12.0 seconds. |
| Sandbox Compose up --build --wait; test_notifications.py; test_notification_outage.py | 0 | Imported and activated the local n8n workflow. Verified receipt deduplication, invalid signature, expired timestamp and independent delivery failure. Restored the sandbox after testing. |
| python3 scripts/evaluate.py | 0 | Generated Markdown and JSON from a fresh, unexposed project and removed its temporary database and Redis volumes. Verified the per-peer limit on the 31st question in a fresh workspace. |
| Gitleaks; govulncheck ./...; npm audit --audit-level=high; pip-audit --path .venv/lib/python3.12/site-packages | 0 | Gitleaks reported no leaks in release sources. The three dependency scanners returned no known findings for their checked inputs. |
| git diff --check | 0 | Clean patch whitespace. |

## Failures resolved during development

- Docker initially exited 1 with a missing Desktop socket. The installed daemon was started. Registry DNS and Docker socket access required normal tool approvals. Native uv panicked in the macOS sandbox (exit 101, Make exit 2) and succeeded with permitted system access. An early pytest invocation before installation exited 1.
- The first invalid-sample probe received 503. Migration 004 added the column-level grant Postgres needs for workspace row locks. The live suite then passed.
- A Python import-order failure and a React effect lint failure each exited 1. Both were fixed. The first browser run exited 1 on ambiguous locators. Accessible names were verified in the captured page and both scenarios passed on rerun.
- A disposable Compose launch exited 1 before a benchmark ran. Its exact failed dependency was not retained. Added startup diagnostics and changed PostgreSQL readiness to check the TCP listener used by the migration job. Fresh-volume reruns passed. An invalid pip-audit flag combination exited 2 before scanning; the supported installed-path invocation passed.

## Files and next action

The implementation is under apps/web, services/gateway and services/ai. The supporting changes are compose.yaml, infra, migrations, fixtures, packages/contracts, workflows, scripts, Makefile, .github/workflows, .env.example and ignore rules. The project memory, README, evaluation reports and docs/nexusai-demo.webm are ready for review.

The optional sandbox is running on this Mac. Open http://localhost:3000 and follow the README walkthrough. To reproduce from stopped services run `make sandbox sandbox-check`. To stop this stack while retaining data run `make sandbox-down`. The worker has no HTTP health endpoint; live job and reclaim tests verified its behavior.

Remaining release decisions are the hosting region, budget, production HTTPS/proxy topology, backup and restore checks, and approval to publish. The current web proxy shares its per-peer allowance. Real-model evaluation, human answer-quality ratings, OCR, private uploads and bilingual extraction are outside the verified release. Remote GitHub Actions and public URL checks require a push and deployment.

## 2026-11-25 · Production M1

The later M1 instruction authorized local implementation on top of audited commit dc8a58d. Migrations 007–008, named memberships, scoped Go transactions and company review are implemented. A separate Compose project passed the database upgrade, authorization and concurrency checks, three browser scenarios and existing workflow regressions. The default project database remained on 006. [Full verification record](docs/PROGRESS.md) · [Local operator and rollback guide](docs/SECURITY.md)
