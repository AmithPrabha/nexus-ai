# Changelog
Record actual changes only. Use Added, Changed, Fixed, Security, Known limitations and Verification for each milestone. Include date and commit if available. Do not label a planned feature as shipped.

## Unreleased — documentation baseline
### Added
- Consolidated proposed product specification, architecture, API contracts, decision register, progress tracker and Codex instructions.
### Changed
- Replaced phase-one-only execution instructions with gated autonomous local development.
### Known limitations
- No application implementation, tests, benchmarks or deployment verified.
- Working name, provider/model, hosting and repository integration remain unresolved.
### Verification
- Documentation reviewed against existing PROJECT_SPEC.md; runtime checks not applicable yet.

## 2026-11-25 — G0
- Inspected the clean baseline and separate portfolio. Restored the missing authoritative project specification from the matching final local development archive (SHA-256 bcf1b647e06f295a522346fb2342e3b95b526dc95561a554e025bfdd541586e3).

## 2026-11-25 — foundation
- Added a Compose stack, locked Go and Python dependencies, versioned migration runner, role-separated database access, mock-labeled AI health endpoint and isolated demo sessions.
- Added five fictional PDF samples, local secret generation, HTTP smoke checks and CI entry points.

## 2026-11-25 — G2
- Implemented the Go-owned job, revision, decision, audit and transactional outbox paths, mock PDF extraction with verified evidence, mock telemetry and local review notifications.
- Implemented decimal validation, optimistic concurrency, idempotency and scoped duplicate detection. Live tests checked five initial samples and lease recovery. Added private job and public OpenAPI documents.

## 2026-11-25 — G3
- Added a replayable seed and a read-only policy service combining pgvector and full-text rankings. The mock answer quotes one assigned clause or abstains. The gateway independently resolves source metadata and rejects mismatched quotations.

## 2026-11-25 — G4
- Added the responsive Next.js workspace, PDF preview, editable review with history, cited policy UI, session metrics, reset and evaluation views. Added a generated TypeScript API client schema and desktop/mobile Playwright acceptance tests. The Next.js production container passed its local health check.

## 2026-11-25 — evaluation and release verification
- Added 20 labeled evaluation PDFs, 30 policy questions, a disposable Compose benchmark and reports with measured outcomes, per-case evidence and nearest-rank latencies.
- Added an optional isolated n8n workflow, signed receipt handling, bounded delivery retries and delivery status in invoice detail. The default mode records local receipts. Added the browser walkthrough and release instructions.
- Verified stream recovery after a lost ACK, denied replay/tampering, workspace isolation, quota enforcement and the generated API client. CI includes migration, integration, browser, evaluation and secret checks.

## 2026-11-25 · Production M1

Added durable company ownership, named local sign-in, role-based approval, scoped PostgreSQL policies, preserved legacy history, organization UI and local integration/browser coverage. The work was verified on isolated synthetic data. [File inventory](docs/CHANGELOG.md) · [Verification](docs/PROGRESS.md)
