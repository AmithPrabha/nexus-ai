# API contracts — proposed baseline
PROJECT_SPEC.md section 8 is authoritative for scope. Implement machine-readable OpenAPI and JSON schemas under packages/contracts; generate/check typed client against them. Details below expand the specification; reconcile before coding and record changes in DECISIONS.md. No endpoints implemented yet.

## Conventions
Public prefix /api/v1; JSON, UUID identifiers, UTC RFC3339 timestamps, decimal strings for money. Session cookie and CSRF on mutations; scoped queries, 404 for inaccessible IDs. Request ID in responses/logs. Error: {"error":{"code":"VALIDATION_ERROR","message":"Safe message","request_id":"..."}}. 400 malformed, 401/403 auth, 404 scope, 409 stale/conflict, 429 quota, 503 dependency unavailable. No stack traces. Cursor pagination with bounded limit (default 20, max 100). Unknown fields rejected on mutation schemas.

| Method/path (public prefix) | Request | Success |
|---|---|---|
| POST /demo/sessions | empty | 201 session_id, expires_at; cookie |
| GET /samples | none | 200 items: sample_id,title,description,preview_url (safe fixture only) |
| POST /invoices | sample_id; Idempotency-Key header | 202 invoice_id,job_id,status |
| GET /invoices | status,cursor,limit | 200 items,next_cursor |
| GET /invoices/{id} | none | 200 invoice,revision,validation_results,version |
| GET /jobs/{id} | none | 200 job_id,status,attempts,error_code (nullable) |
| PATCH /invoices/{id}/review | version,fields,reason | 200 updated invoice/revision/validation |
| POST /invoices/{id}/decision | version,decision (approve/reject),reason | 200 invoice_id,status,version |
| POST /invoices/{id}/retry | proposed version + Idempotency-Key | 202 invoice_id,job_id,status |
| GET /invoices/{id}/audit | cursor,limit | 200 items,next_cursor |
| POST /policy/questions | question (1–1000 chars) | 200 answer,abstained,citations,telemetry |
| GET /metrics/demo | none | 200 measured counts,latency,tokens,cost or null,mode |
| GET /evaluation | none | 200 versioned benchmark or explicit not_run status |

Health endpoints outside prefix: GET /healthz, /readyz; 200 healthy/ready or 503 not ready. Do not leak dependency credentials.

## Extraction payload
```json
{"supplier_name":"Example Trading LLC","supplier_trn":"100123456789012","invoice_number":"DEMO-001","invoice_date":"2026-01-01","currency":"AED","subtotal":"100.00","vat_rate":"0.05","vat_amount":"5.00","total":"105.00","tax_treatment":"standard","evidence":{"subtotal":{"page":1,"excerpt":"Subtotal AED 100.00"}},"warnings":[]}
```
Fictional example. Fields nullable when missing; evidence map for extracted fields; exact excerpts verified against page text. tax_treatment standard/zero/exempt/mixed/unknown. VAT rate expressed as fraction. Validation result: rule_code,severity,expected,actual,message. Review fields allow only editable extraction fields, not status or evidence; preserve original extraction. Reject stale version with 409. Require reason for rejection/override. Never approve via model.

## Internal service contracts
Private network + service credential; reject unauthorized/stale attempts. Proposed POST /internal/jobs/{id}/claim returns attempt and lease token/expiry for eligible job; atomic claim and state transition. POST /internal/jobs/{id}/complete accepts attempt,lease_token,extraction,telemetry; POST /internal/jobs/{id}/fail accepts attempt,lease_token,error_code,retryable,telemetry. Duplicate completion returns same durable result; stale attempt 409. Worker ACK only after durable accepted result or confirmed terminal/stale disposition. Redis message: event_id,job_id,workspace_id,document_id,schema_version; no secrets or PDF text. Gateway determines retry eligibility and outbox scheduling. Internal FastAPI /extract and /retrieve-answer accept scoped document/policy context and request ID; /health reports service liveness. Worker may call extraction library directly. Retrieval output: answer,abstained,citations [{chunk_id}],reason; gateway resolves page/clause/excerpt from trusted metadata. Telemetry includes mode,provider,model,latency_ms,input_tokens,output_tokens,estimated_cost_aed (nullable when unavailable).

## Notification
HMAC signed event with event_id,timestamp,invoice_id,state,reason_codes,demo flag; replay window and receiver deduplication. Public demo logs sandbox notification only. No approval endpoint in n8n. Version payloads and test signature, replay, duplicate and failure cases.

## Contract tests
Idempotency same key/same body vs changed body; session isolation; invalid sample; stale version; stale worker attempt; retry exhaustion; invalid citation; quota; unavailable provider; no real notifications. OpenAPI must reflect actual behavior, not merely this draft.
