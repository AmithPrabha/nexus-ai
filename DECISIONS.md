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
Actual repository layout, compatible pinned versions, provider/model and embedding dimensions, hosting/budget, production origin/cookie topology, final brand name, private upload licensing/security review. Record decisions when resolved; do not invent credentials or availability.
