# Nexus AI production migration plan

**Original proposal at the audit baseline.** Two fictional companies should be able to use the existing invoice workflow with named users, enforce who can approve, and retain invoice history after sign-out. This proves the ownership model before connecting a document bucket or changing job delivery.

The baseline is `dc8a58d`. This plan keeps Go as the business API, Python as the processing service, Next.js as the review UI and PostgreSQL as the system of record. Milestones below are proposed. The later M1 instruction authorized local implementation and verification. [Current architecture](ARCHITECTURE.md) · [Component dispositions](PRODUCTION_GAP_ANALYSIS.md) · [Decision register](DECISIONS.md)

## Current delivery status

M1 has been implemented locally. Its acceptance evidence and remaining production decisions are recorded in [progress](PROGRESS.md) and [security](SECURITY.md). The next proposed implementation is M2, private document intake, under separate authorization. SQS and AWS deployment retain their later gates.

## M1 · Durable tenants and attributable decisions

Run the existing sample invoice from submission through human approval for two separate fictional tenants. Keep the anonymous public demo as its own mode. Use a local identity fixture for automated tests and define the integration boundary for a production identity provider.

1. Capture the current sample-contract, decision, expiry and job-recovery scenarios as the compatibility baseline. Define tenant, user, membership and action permissions. Identify users by verified issuer and subject. Resolve the active tenant server-side and check membership on each protected action.
2. Add migrations after 006. Separate durable ownership from login expiry. Attach tenant ownership to business records and enforce it across relationships, including jobs, revisions, idempotency entries and policy assignments. Scope demo cleanup and the global policy-assignment trigger before creating customer-shaped records. The legacy purger and demo database role must have no path to delete them.
3. Use tenant-scoped queries plus database constraints and row-level policies on customer data. Set tenant context inside a transaction when using pooled connections. Test both Go and the Python policy reader under their actual restricted roles. Give internal jobs scoped service authority and persist the effective principal.
4. Add role checks for submit, edit, decide and override. Audit the named actor, tenant, affected revision, decision, reason and ruleset version. Agree the override or second-approver policy before enabling it. Distinguish historical demo actors from new users. Keep tenant data out of demo metrics, lists, previews and reset.

**Exit gate.** For tenants A and B, users can list only their own invoices, a reviewer can correct, and only the approved role can decide. A user in both tenants must act within the explicitly selected one. Test cross-tenant requests for every resource, membership revocation, stale and concurrent decisions, reassigned pooled connections, service callbacks and audit access. Expire a session, purge an unrelated demo and sign in again. The customer invoice, named decision and revisions must survive. Rerun the existing demo and contract suites.

**Rollback.** Disable customer entry points while investigation proceeds. Keep new durable records and audit rows intact. Use additive schema and compatibility tests with the previous demo binary. A release that predates tenant ownership may serve only the isolated demo.

## M2 · Private document intake with a replayable source

Submit a synthetic private PDF, scan it, extract it, authorize preview and reproduce the review from the exact accepted bytes. Develop against local storage fixtures or an S3-compatible test environment. Verify AWS-specific IAM and object semantics later in approved staging.

- Add a tenant-owned document ID and an object manifest recording bucket/key, version, checksum, size, media type and quarantine state. Add an authenticated intake/finalize contract. Generate keys on the server, enforce upload limits, use a private encrypted bucket and short-lived grants. Finalization verifies the observed object metadata, content and version, records the result idempotently, and releases work only after scanning. Presigned URLs can be reused until expiry, and an upload to an existing key can replace its contents. Pin the exact version for scan, extraction and preview. [AWS S3 presigned URL behavior](https://docs.aws.amazon.com/AmazonS3/latest/userguide/using-presigned-url.html)
- Give Go and Python a document-reading boundary with a fixture implementation. Authorize every private preview and release a time-limited read of a specific accepted version. Isolate untrusted parsing with enforced process, memory, time and page limits. Establish retention of accepted, rejected, abandoned and derived data.
- **Exit gate.** Test mismatched checksums, replacement after finalization, duplicate finalize calls, oversized/unreadable PDFs, failed scans, interrupted uploads, parser termination and cross-tenant read denials. Verify that the same bytes underlie the reviewer preview and saved evidence. Preserve the five-sample demo.
- **Rollback.** Disable new intake. Preserve accepted object versions and manifests for review, reconciliation and retention. Existing invoices continue to use their original source adapter.

## M3 · SQS delivery behind the existing job contract

Use one outbox and lease protocol for Redis-backed demo jobs and SQS-backed production jobs. Choose the transport when creating the job. Queue messages carry opaque identifiers and a schema version. Go derives tenant and document access from the database claim.

- Add an SQS publisher and consumer with long polling and a dead-letter queue. Track publication and recovery in PostgreSQL. Recover a lost publish without republishing every queued job on the current ten-second cadence. Multiple sends can occur around a lost response, so keep the stable job ID and fenced completion transaction. [AWS standard queue delivery](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/standard-queues-at-least-once-delivery.html)
- Align receive visibility, the processing deadline and the DB lease. Bound or renew both when processing runs long. Delete a message after a durable outcome or verified terminal disposition. For busy, deferred and stale deliveries, specify who owns the next retry before acknowledging. Track SQS receive count separately from accepted DB attempts. Reconcile dead-lettered messages against actual job state before authorized redrive. [AWS visibility timeouts](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html) · [AWS dead-letter queues](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-dead-letter-queues.html)
- **Exit gate.** Under both transports, exercise publish-then-crash, duplicate delivery, worker death before and after commit, expired lease, callback timeout, unavailable queue, retry exhaustion and manual redrive. Verify a single committed revision for a successful job, observable failed work, and no lost accepted submissions. Keep Redis quota tests green independently.
- **Rollback.** Route newly created jobs back to Redis. Drain and reconcile existing SQS jobs using their recorded transport. Keep both readers available through reconciliation and compare counts by job ID and final DB state before retiring an adapter.

## M4 · Qualify extraction and financial review for the launch scope

Define a support matrix for document layouts, languages, currencies and tax treatments with evidence from a representative labeled set. Select OCR or a model only where needed for that scope. The mock provider remains the CI default.

- Version extraction, provider, configuration and applicable Go rules. Extend the mock-only DB constraint, API types, review labels and telemetry together. Record provenance per field, uncertainty and review overrides. Preserve original OCR/text evidence and human corrections as separate revisions.
- Evaluate field accuracy, missing values, evidence validity, incorrect clean routing, review volume, latency, resource use and any provider cost. Include scans, ambiguous fields and unsupported layouts. Agree thresholds with the business before testing for launch. Keep unsupported cases reviewable. Private policy ingestion, Arabic support and external notifications can be scheduled separately unless required for the pilot.
- **Exit gate.** Domain owners sign off on supported rules and allowed overrides. A representative offline holdout passes the agreed thresholds, with results split by document type. Demonstrate that extraction alone cannot approve an invoice. Paid provider trials and customer documents require explicit authorization.
- **Rollback.** Disable the new provider for future jobs and route unsupported work to human review. Preserve model/ruleset versions on historical results and label any mock operation explicitly.

## M5 · Release engineering and an authorized AWS staging rehearsal

Build a reproducible deployment and test the recovery procedure. Prepare infrastructure and deployment changes only after authorization for implementation. Provision AWS staging only after approval of region, costs and environment.

- Package the existing images for Fargate. Configure one public HTTPS origin, private API and worker networking, encrypted database/Redis connections, managed secrets and separate runtime roles. Keep elevated migration privileges in a one-shot job, remove production dependence on fixture mounts, and restrict the private callback listener. Give AWS access to each workload through its task role. Give image pulling and task startup their own execution role. [AWS ECS task IAM roles](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-iam-roles.html)
- Provision approved managed PostgreSQL/pgvector, private S3, SQS/DLQ and a quota backend. Add image and dependency scanning, immutable image promotion, additive migration checks and gated releases. Extend instrumentation as each earlier milestone lands. Correlate request, job, attempt and document IDs without logging invoice text, credentials or signed URLs.
- **Exit gate.** Exercise least-privilege identities, secret rotation, private-only callbacks, database point-in-time restore and recovery of matching object versions. Measure worker progress, backlog age, DLQ count, retry/error rates, decision latency and tenant load. Rehearse scaling under contention, alarms, rollback and recovery against agreed service objectives with synthetic data. Verify old/new worker compatibility during a rolling release.
- **Rollback.** Pause new intake, preserve job intent and audit evidence, drain or reconcile in-flight jobs, then restore the last compatible image/configuration. Reconcile DB and document versions in a separate recovery environment before switching traffic.

## M6 · Limited customer pilot and release decision

Enable intake for an approved pilot tenant only after M1–M5 pass for its support matrix. Verify permissions, real identity integration, retention policy, quality, cost and support ownership. Compare observed results to the agreed thresholds and exercise the stop-intake switch. Expand by tenant after review. Release approval, AWS deployment and customer onboarding are separate from approval to start local M1.

## Compatibility checklist for every milestone

| Boundary that changes | Migration rule |
|---|---|
| Browser and public API | Preserve demo session/CSRF, `POST /api/v1/invoices` with `sample_id`, HTTP 202 and stable IDs, decimal strings, pagination and 409 conflicts. Add an explicitly versioned or additive customer intake contract. The current Invoice response requires `sample_id`; introduce a document-aware response before creating private invoices. Test existing clients against upgraded services. |
| Database and historical data | Expand first, inventory and backfill eligible rows in restartable batches, reconcile counts/owners, validate constraints, then switch reads/writes. Keep demo rows explicitly demo-owned. New actor and provenance fields on old records must preserve their known historical meaning. Retire old fields only in a later approved release after all readers and jobs have moved. |
| Private worker and queue | Today's stream field `document_id` contains a sample key and a claim returns `sample_id`. Introduce a versioned source reference and negotiate/deploy readers before producing the new shape. Keep legacy consumers bound to compatible jobs. Preserve fencing, terminal replay and per-scope idempotency semantics. |
| Retrieval and observability | Keep shared demo policies separate from customer assignments, protect citation lookup in both services and version any new embeddings. Keep mock benchmark and tenant operational metrics distinct. Disallow sensitive document content in traces and diagnostic labels. |

**M1 acceptance scenario.** A successful demonstration ends with tenant A's named approver approving an invoice, tenant B being denied access, the decision surviving logout and cleanup, and the original demo passing its regression suite.
