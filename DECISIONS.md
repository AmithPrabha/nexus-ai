# Architecture decision register
All entries below are proposed baseline decisions inherited from or expanding PROJECT_SPEC.md; not evidence of implementation. User approval required for material deviations. Append new ADRs with date, status, context, alternatives, decision, consequences and verification; supersede rather than silently rewrite history.

| ID | Decision | Rationale / trade-off |
|---|---|---|
| ADR-001 | Go gateway + Python AI service | Demonstrate distributed backend and AI specialization; two runtimes increase operations overhead. No .NET/NestJS duplication. |
| ADR-002 | PostgreSQL + pgvector | One authoritative store and hybrid retrieval; embedding dimension/version must match. |
| ADR-003 | Redis Streams + transactional outbox | Reliable asynchronous processing; at-least-once delivery requires deduplication and recovery. |
| ADR-004 | Gateway owns invoice writes and decisions | Deterministic validation and human approval; AI/n8n cannot authorize payments. |
| ADR-005 | Sample-only public demo | Lower privacy/security exposure; real uploads deferred. |
| ADR-006 | Provider adapter and mock CI | Reproducible tests without spend; real AI quality requires separate evaluation. |
| ADR-007 | Limited UAE standard-rate arithmetic | Local relevance without claiming certified tax compliance; unsupported cases reviewed. |
| ADR-008 | Separate monorepo, portfolio link | Protect existing website; actual integration depends on repository inspection. |
| ADR-009 | Custom RAG, optional n8n | Avoid premature agent frameworks; deterministic workflow sufficient for MVP. |
| ADR-010 | Phase gates with autonomous local execution | Reduce interruptions; stop for risky actions or unresolved blockers. |
| ADR-011 | Proposed internal job claim endpoint | Explicit attempt fencing for worker recovery; validate against actual implementation and OpenAPI. |

## Pending decisions
The local layout and dependency locks are in the checkout. Mock embeddings use 96 dimensions. Remaining decisions cover a real provider/model, hosting and budget, production origin/proxy topology, final brand name, and private-upload licensing and security.

### ADR-012 · Local foundation · 2026-11-25
The seven repository documents matched the final development archive byte-for-byte, so its missing PROJECT_SPEC.md was restored verbatim. Use an independent Next.js application in this repo and reserve portfolio changes for release approval. The gateway verifies a single exact browser origin and uses an opaque random session secret stored only as a digest in Postgres. This supports revocable, expiring anonymous workspaces and same-origin UI proxying. The migration job owns schema changes and runtime roles receive explicit grants. Verify with the Compose migration replay and HTTP smoke tests.

### ADR-013 · Mock retrieval · 2026-11-25
Use deterministic 96-dimensional hashed token vectors in mock mode. Store the index version, scope each query with workspace assignments, fuse vector and keyword ranks, and quote a single clause exactly. This provides reproducible offline retrieval and lets the gateway independently verify the entire mock answer. The short fictional clauses retain their natural boundaries. The mock cannot establish real embedding or language-model quality. Verify with the policy integration suite and the separately labeled benchmark.

### ADR-014 · Signed sandbox receipts · 2026-11-25
Use the pending-review transaction to create delivery intent. Keep delivery retries and failures separate from the invoice state machine. The optional self-hosted n8n webhook delegates HMAC verification and event deduplication to the same private Go receiver used by the default local sink. This keeps service credentials outside the workflow and gives concurrent receivers a database uniqueness guarantee. The workflow can reach only the private Docker network. Test valid, repeated, expired and modified events plus an unavailable workflow.

### ADR-015 · Durable tenant ownership and named approvals · 2026-11-25

Implemented for local M1. Retain the existing scope UUIDs and job contract, separate company ownership from session lifetime, and authorize a named membership inside the invoice transaction. Adopt the local one-time code boundary and explicit override role policy. [M1 decisions](docs/DECISIONS.md) · [Security model and rollback](docs/SECURITY.md)
