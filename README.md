# NexusAI

NexusAI is a fictional UAE accounts-payable workspace. Select a sample PDF, watch its job complete, inspect the evidence and decimal checks, correct a discrepancy and record a simulated approval or rejection. The policy assistant quotes assigned handbook clauses and abstains when evidence is insufficient.

[Browser walkthrough](docs/nexusai-demo.webm) · [Measured mock evaluation](evaluation/REPORT.md) · [Architecture](ARCHITECTURE.md) · [Public OpenAPI](packages/contracts/openapi.json)

## Run the demo

Install Docker with Compose v2.24.4 or newer, Python 3 and Make. Start Docker, then run the following from the repository root.

```sh
make setup
make up
make smoke
```

Open [the local dashboard](http://localhost:3000). Use localhost in the browser to match PUBLIC_ORIGIN. The gateway also listens on 127.0.0.1:8080. Setup generates a private .env once, with independent random database credentials and a service token. Retain it with the named database volumes. `make down` stops the default stack and preserves data. The private stores, worker listener and AI service have no published ports.

The default demo uses a deterministic mock adapter. It accepts five fictional sample PDFs, requires human action for every decision and records sandbox receipts locally. Reset demo expires the visitor's workspace. There is no payment operation or external notification destination.

## Recruiter walkthrough

1. Open **Invoice auditor**, choose **Design services** and process the sample. Compare the source PDF, printed field evidence and VAT discrepancy.
2. Set VAT amount to **24.00** and total to **504.00**. Enter a reason and save the correction. Simulate approval and inspect the revision timeline and audit actor.
3. In **Policy assistant**, ask who approves purchases above AED 10,000. Expand the cited procurement clause. Ask about the current UAE corporate tax rate to see an abstention.
4. Open **Evaluation** for the measured mock benchmark. **Architecture** links to the public API and source repository.

## Reproduce verification

| Command | Coverage |
|---|---|
| `make test lint` | Go unit tests, vet and formatting, plus locked Python tests and Ruff. Requires Go 1.27, Python 3.12 and uv. |
| `make web-check` | Install from the npm lockfile, lint, type-check and check the generated client against OpenAPI. Use Node 24 or the tested Node 26 runtime. |
| `make migrate seed smoke` | Replay migrations and policy indexing, then check session bootstrap, expiry on reset, exact origin, health and all PDF previews. |
| `make integration policy sandbox-check` | Live worker, stale leases, pending stream reclamation, idempotency, terminal decisions, policy scope, quotas, signed receipts and replay checks. |
| `cd apps/web && npx playwright install chromium && npm run e2e` | Desktop correction/approval/audit and mobile citation/abstention/quota tests. For installed Google Chrome use `PW_CHANNEL=chrome npm run e2e`. |
| `make eval` | Run 20 labeled invoices and 30 labeled questions through the real HTTP and queue paths in a fresh temporary Compose project. Writes Markdown and per-case JSON under evaluation/. |
| `make security` | Scan Git-eligible sources including uncommitted files with redacted Gitleaks output. |

Local Compose applies migrations and seeds on startup. Integration tooling uses a separate test-only Compose override for synthetic lease and access-control fixtures. Benchmark clients receive ordinary quotas. Repeated live checks share the current stack's limits; CI starts with a fresh database and Redis. GitHub Actions is configured in [.github/workflows/ci.yml](.github/workflows/ci.yml). Remote CI execution requires pushing the changes.

## Optional n8n sandbox

Run `make sandbox sandbox-check` to use the pinned n8n container and exercise an actual review notification. This starts the same local demo with a private, internal-only workflow network. The import and activation jobs load the checked-in workflow using the self-hosted CLI. The workflow forwards a signed event to exactly one private receipt endpoint. The gateway verifies HMAC and freshness and deduplicates by event ID. The workflow has no database passwords, service token, approval action, public port or external network. `make sandbox-down` stops the complete optional stack and retains volumes. [Workflow definition](workflows/sandbox-review.json) · [n8n CLI reference](https://docs.n8n.io/deploy/host-n8n/configure-n8n/use-the-command-line.md)

Delivery retries independently of invoice processing. The invoice detail reports pending, recorded or failed delivery. To reproduce the outage test on a dedicated local demo, stop only the n8n service using the sandbox Compose override, run the integration service with /checks/test_notification_outage.py, then restore the sandbox with `make sandbox`.

## Evaluation boundaries

The benchmark uses labeled fictional documents and questions. Eighteen English text PDFs share a printed-label layout. The scanned and Arabic cases are scored for routing to review. The report separates missing fields, per-rule precision/recall, citation validity, expected clauses, abstention and latency. Real model quality, token consumption, cost and human-rated answer support require separate evaluation. The financial rules illustrate a narrow standard-rate case and the amount threshold is fictional company policy. [Full per-case results](evaluation/report.json)

## Release and portfolio handoff

The external release gate requires approval. The proposed topology is Vercel for the Next.js UI, container hosting for the gateway, AI API and worker, and persistent managed PostgreSQL/pgvector and Redis. Keep the mock provider for the first public demo. Proposed pilot spending ceiling for discussion is **US$60 per month** with zero paid model calls. Exact regional compute, storage, backup and egress charges must be priced before provisioning. [Vercel plans](https://vercel.com/docs/plans/hobby) · [Render services and pricing](https://render.com/pricing)

Before a public release, configure the final HTTPS origin and proxy rewrite, generate production secrets, restrict internal listeners, use TLS for managed stores, configure backups and test restoration. Bundle the five sample PDFs and the evaluation JSON at their documented runtime paths. Verify cookie security, CSRF, quotas behind the actual proxy, cross-session denial, expiry and human approval at the final URL. Then link the separate Next.js portfolio project entry or /ai-lab route to that verified URL, the source and benchmark. Private uploads, OCR, bilingual extraction, real provider calls and enterprise identity require separate work.

## Organization review

M1 adds local named accounts and durable company invoice history. Open `/organization` on the dashboard and enter a code issued by your operator. REVIEWER can approve clean invoices. An OWNER or ADMIN can approve unresolved findings with a reason. The public demo remains available at the root. Follow the [local account setup and rollback guide](docs/SECURITY.md) and the [milestone verification record](docs/PROGRESS.md).
