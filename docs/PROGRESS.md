# Production migration status

**25 November 2026 · M1 implemented and verified locally. M2 awaits authorization.**

## M1 result

A named Company A reviewer processed a sample through Redis and Python, corrected the source fields, and approved it. Company B could not list, read, correct, retry, approve or inspect that invoice or job. Independent concurrent approve/reject requests returned one success and one 409. The decision retained the correct actor UUID, display name, role, rule version and revision across sign-out, expiry, membership revocation and demo cleanup. A separate browser scenario signed in as Alex Reviewer and reopened an approved invoice. All data used for verification was fictional.

| Verification on the M1 working tree | Result |
|---|---|
| Upgrade from 006, migration replay, legacy cleanup rehearsal | Exit 0. Created a pre-upgrade approved invoice in an isolated Compose volume. Migration 007 preserved ownership, invoice and audit identity. Migration 008 subsequently hardened credential and revision constraints. Replay succeeded. Removing its demo session inside a rehearsal transaction preserved the historical approval and anonymous actor. The original local database was checked separately and still recorded 006. |
| Gateway | Exit 0 for Go race tests, vet and formatting. Public and internal router coverage agrees with OpenAPI. |
| Python | Eight tests passed with six dependency deprecation warnings. Ruff passed. |
| Web and contract | Production Next.js Docker build, ESLint, TypeScript, generated public schema check all exited 0. |
| Live security suite | Exit 0 for named login, replay/expired code denial, CSRF, two companies, multi-company membership, viewer denial, reviewer correction and clean approval, owner override, revocation, expiry, logout, concurrent decisions, scoped PostgreSQL reads and preserved audit history. A direct database check also rejected a revision author from the other company. |
| Browser | All three Playwright scenarios passed with Chrome. Named sign-in and approval, original desktop correction/approval and mobile policy/quotas completed in 12.0 seconds on the final browser run. |
| Existing workflow regressions | Exit 0 for demo smoke, five sample invoice workflows, VAT checks, lost-ack recovery, lease fencing, policy isolation, signed sandbox receipts, optional n8n and its outage scenario. |
| Mock evaluation | Exit 0 in an isolated copy and disposable evaluation project. Correct routing for 20/20 invoices, 177/177 populated fields on 18 text fixtures, expected clauses for 20 answerable questions and abstention on 10 unsupported questions. Checked-in historical benchmark files were preserved. |
| PostgreSQL restore | Exit 0. A logical backup of the isolated M1 database restored into a separate database. Source and restore matched on 8 migrations, 24 invoices, 97 audit events and 7 named approvals. This was a local logical restore in the same cluster. |
| Pause rehearsal | A second gateway with organization access disabled denied a previously valid session (401) and login (403), accepted a demo session, and left the original organization session usable on the enabled gateway. |

The operator CLI was also exercised end to end. It issued a one-time code for a real test membership, which the gateway redeemed and then signed out. Gitleaks reported no leaks in the scanned working tree.

The browser test initially exposed a successful sign-in that changed only the URL fragment and left the form on screen. Reloading the organization route after authentication fixed it. The fresh-project browser fixture now resolves the Compose file explicitly. The upgrade suite also checks the exact historical demo audit response shape. A JSX syntax error and navigation lint issues were corrected before the final build and browser run.

### Reproduce in a separate local database

From the repository root, choose an unused Compose project name and free ports. The following commands target only that project. The authenticated browser test creates synthetic records in the selected local PostgreSQL service. The existing public demo can keep its usual ports.

```sh
make setup
export COMPOSE_PROJECT_NAME=nexusai-m1-local
export COMPOSE_FILE="$PWD/compose.yaml:$PWD/infra/m1-check.compose.yaml:$PWD/infra/checks.compose.yaml"
export PUBLIC_ORIGIN=http://localhost:13317
docker compose up -d --build --wait web ai-api ai-worker
docker compose run --rm migrate
GATEWAY_URL=http://127.0.0.1:18317 python3 scripts/smoke.py
docker compose run --build --rm integration /checks/test_invoices.py
docker compose run --rm integration /checks/test_policy.py
docker compose run --rm integration /checks/test_tenants.py
make test lint web-check
(cd apps/web && WEB_URL="$PUBLIC_ORIGIN" NEXUS_E2E_PROJECT="$COMPOSE_PROJECT_NAME" npm run e2e)
```

For another concurrent isolated project, set `M1_WEB_PORT` and `M1_GATEWAY_PORT` before startup and match `PUBLIC_ORIGIN` and `GATEWAY_URL` to those ports. Use installed Playwright Chromium or set `PW_CHANNEL=chrome` for installed Chrome. `docker compose stop` stops the selected local environment and retains its records. See the [operator walkthrough and rollback instructions](SECURITY.md) and [M1 file inventory](CHANGELOG.md). Hosted CI, production identity delivery and AWS deployment were outside this local verification.

## Historical baseline

The read-only inspection began on a clean `main` at `dc8a58d`. The local `origin/main` reference pointed to that commit. The implementation, contracts, dependency locks, evaluation results and video are tracked. The older handoff describes an uncommitted checkout at `2bdb5e9`. That statement dates the earlier verification run. The root specification's “proposed” heading likewise predates the implemented local workflow. [Historical handoff](../PROGRESS.md) · [Original specification](../PROJECT_SPEC.md) · [Changelog](../CHANGELOG.md)

## Original audit scope

The baseline contained 117 tracked paths. The inventory covered application source and tests, manifests and locks, generated API types, all three OpenAPI contracts, six SQL migrations, database grants, Dockerfiles, Compose and overrides, Make targets, scripts, CI, n8n workflow, fixtures, evaluation records, video and root project instructions. Generated files and binary assets were inventoried as artifacts. The public schema in the source tree is a symlink to the canonical contract. The Dockerfile copies that contract into the built site. [Repository map](ARCHITECTURE.md) · [Web image](../apps/web/Dockerfile)

## Evidence from the original audit

| Verification | Result |
|---|---|
| Go gateway, `go test -race -count=1 ./...` | Exit 0. Package passed, including rule cases, routing boundaries, OpenAPI route enumeration, policy abstention and signed-event tests. |
| Go gateway, `go vet ./...` and gofmt clean check | Exit 0. |
| Python service, `python -m pytest -q -p no:cacheprovider` in the existing virtual environment | Exit 0. Eight tests passed with six dependency deprecation warnings from the SWIG bindings and Starlette test client. Covers sample evidence, schema retry, PDF limits and FastAPI contract equivalence. |
| Python service, `ruff check --no-cache .` | Exit 0. |
| Web, `tsc --noEmit --incremental false`, `npm run lint`, `npm run contracts -- --check` | All exited 0. Generated TypeScript matches the public contract. |
| Direct Python extraction, mock mode | Exit 0. Design services yielded ten evidence fields including printed VAT 20.00. The evaluation scan yielded zero evidence fields and an OCR-required warning. |
| Running local environment, `docker compose ps` and HTTP GETs | Exit 0. Web, gateway, AI API, PostgreSQL, Redis and optional n8n reported healthy. The worker reported running with no container health status. Gateway readiness returned ready and the UI root returned HTTP 200. These checks describe the existing containers at observation time. |
| PostgreSQL catalog and migration ledger in a read-only transaction | Exit 0. All six tracked migrations were recorded, the vector extension reported 0.8.1 and zero tables in the public schema had row-level security enabled. |

The live checks did not submit invoices or mutate the running demo. The route test compares methods and paths. The Python schema and frontend generation checks cover different contracts. None of those checks alone exercises a complete financial decision. [Go contract test](../services/gateway/contracts_test.go) · [Python contract test](../services/ai/tests/test_health.py) · [Browser tests](../apps/web/tests/workspace.spec.ts)

## Historical evidence and its limits

- The G0–G5 ledger records Compose migration replay, invoice/worker recovery, policy isolation, browser flows, security checks and sandbox notification outage testing. The push/PR workflow runs local verification, builds the Compose services, tests browser flows, exercises the sandbox and runs the mock evaluation. Its security target invokes Gitleaks. The historical dependency scans are separate from that target. [Verification ledger](../PROGRESS.md) · [CI definition](../.github/workflows/ci.yml) · [Make targets](../Makefile)
- The checked-in mock benchmark reports correct routing for 20/20 fictional invoices and 177/177 populated fields on 18 template-based text invoices. Its scan and Arabic cases score routing only. It records the expected clause for 20 answerable policy questions and abstention for 10 unsupported ones. Two client workloads ran concurrently, with a single sequential client per workload. Production accuracy, throughput, cost and recovery objectives remain unmeasured. [Method and results](../evaluation/REPORT.md) · [Per-case records](../evaluation/report.json)
- Browser E2E, full Compose integration, clean-volume evaluation, build, dependency scans and an AWS environment were outside this audit's fresh runs. Hosted CI status and cloud deployment status were not independently verified. The tracking above distinguishes source coverage, historical runs and fresh observations.

## Original audit handoff

The earlier audit delivered five planning documents and recommended local M1. The current M1 results appear at the top of this page. AWS provisioning, customer data intake and provider spending have later, separate gates. [Decision register](DECISIONS.md) · [Milestones and acceptance criteria](PRODUCTION_MIGRATION_PLAN.md)
