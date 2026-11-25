# NexusAI API contracts

The public [OpenAPI document](packages/contracts/openapi.json) describes the Go gateway. The [private gateway contract](packages/contracts/jobs.openapi.json) describes the worker and signed receipt routes. The [FastAPI contract](packages/contracts/ai.openapi.json) describes the AI service. The dashboard uses openapi-fetch and [generated TypeScript types](apps/web/lib/schema.d.ts). After changing a schema run `cd apps/web && npm run contracts`. CI checks the generated file with `npm run contracts -- --check`.

## Public API

Routes use /api/v1, JSON and UUID identifiers. Monetary fields are nullable decimal strings. All responses have an X-Request-ID. Errors use an error object containing code, message and request_id. Invalid input returns 400, absent sessions 401, failed request verification 403, inaccessible IDs 404, stale versions or conflicting idempotency keys 409, exhausted quotas 429 and dependency failures 503.

| Action | Method and path | Contract |
|---|---|---|
| Start or resume | POST /demo/sessions | Exact Origin. Returns 201 with session_id, expires_at and csrf_token. Creates an HttpOnly SameSite=Lax cookie on first use. |
| Reset | POST /demo/reset | Expires the current workspace. |
| Select and preview | GET /samples, GET /samples/{sample}/preview | Fixed fictional catalog and session-protected PDFs. |
| Enqueue | POST /invoices | sample_id and Idempotency-Key. Returns 202 with invoice_id, job_id and status. A matching replay returns the original response. |
| Inspect | GET /invoices, GET /invoices/{id}, GET /jobs/{id} | Lists accept a status, cursor and bounded limit (1–100, default 20). Detail includes version, revision and validation results. |
| Correct | PATCH /invoices/{id}/review | version, editable fields and nonempty reason. Saves a new revision, invalidates evidence on edited fields and reruns rules. |
| Decide | POST /invoices/{id}/decision | version, approve or reject, and a reason for rejection or overriding warnings. Final states are terminal. |
| Retry | POST /invoices/{id}/retry | Failed invoice, current version and Idempotency-Key. Starts a new job. |
| Inspect history | GET /invoices/{id}/audit | Ascending paged timeline with actor, state, reason, timestamp and request ID. |
| Ask policy | POST /policy/questions | 1–1000 characters. Returns answer, abstained, trusted citations and measured mock telemetry. |
| Inspect metrics | GET /metrics/demo | Current workspace counts, mean operation latency and sandbox notification count. Unknown token and cost fields are null. |
| Inspect evaluation | GET /evaluation | Versioned public benchmark artifact or an explicit not_run status. |

All scoped mutations require the configured Origin and X-CSRF-Token. Reads require the workspace cookie. Unknown JSON fields on mutations are rejected. Anonymous /healthz and /readyz are outside the API prefix. Readiness verifies the migration ledger and Redis. AI health is checked independently by Compose.

## Private services

The gateway private listener accepts a service Bearer credential on port 8081. A claim returns claimed with sample_id, attempt, lease_token and expires_at, or a busy/terminal disposition. A completion carries the matching attempt, lease, measured latency and extraction (fields, evidence and warnings). Failure adds a bounded error code and retryable flag. Stale attempts return 409. Replaying a durable completion preserves the same revision. Each Redis message carries event_id, job_id, workspace_id, document_id (sample key) and schema_version.

Internal FastAPI exposes /health, authenticated /extract with sample_id and authenticated /retrieve-answer with workspace_id and question. The worker uses the same extraction library directly. The gateway resolves each proposed chunk ID against its assigned policy and checks the quoted text. Simulated review notifications carry reason codes and a unique invoice/version pair. Invoice detail includes sandbox_notification (null, PENDING, RECORDED or FAILED). The independent dispatcher can use an isolated n8n webhook. The private receiver verifies the signed envelope and deduplicates durable receipts. No public notification or approval webhook is present.
