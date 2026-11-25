# Production architecture decision register

**25 November 2026 · Proposed for review against commit `dc8a58d`**

The existing decision register describes the portfolio demo. Its Go/Python split, database ownership, human approval, sample-only public scope and mock CI are reflected in source. The original specification mentions possible Azure work in a later release. This audit evaluates the requested AWS target. The target changes production scope, and the following proposals require approval before implementation. [Original decisions](../DECISIONS.md) · [Original scope](../PROJECT_SPEC.md) · [Observed architecture](ARCHITECTURE.md)

| Proposal | Decision requested and reason | Consequence and verification |
|---|---|---|
| P1 · Retain the repository and service owners | Keep Go/Chi as business API and sole invoice decision writer, Python as processing service, Next.js as review UI, PostgreSQL as authority and OpenAPI as the client boundary. A Go-to-NestJS rewrite would reproduce functioning rules, concurrency controls and contracts while leaving the tenancy and storage gaps open. | Extend the existing services in place. Use the existing demo, contract checks and invoice recovery scenarios as regression gates at every milestone. |
| P2 · Separate customer lifetime from login lifetime | Introduce durable tenants, users, memberships, scoped sessions and named service principals. Keep expiring demo ownership explicitly separate. Enforce membership and action permissions in Go, with database constraints and tenant policies as a second layer. | More schema and authorization tests. Require two-tenant isolation, role revocation and preservation of customer history across logout, expiry, reset and cleanup. |
| P3 · Identify the source document by version | Add a tenant-owned document record and private S3 storage. Issue server-selected upload targets only after authorization. Bind size, checksum and accepted object version to each invoice. Keep the local fixture reader for demo and CI. | Introduces upload finalization, quarantine, preview authorization and retention. Test interrupted upload, repeated finalization, object replacement and expiry. |
| P4 · Retain the database job protocol, move production delivery to SQS | Keep the job ID, outbox, claim, attempt token and transactional completion. Select one delivery backend for a job's lifetime. Retain Redis quota handling during transport migration. | Two transports coexist during rollout. Prove duplicate delivery, failed publish, consumer loss, delayed retry and dead-letter reconciliation. Require a drain and rollback rehearsal. |
| P5 · Keep human authority explicit | Keep financial arithmetic in Go. Version the applicable rules and persist the authenticated actor, decision, reason and authority to override. Keep the mock extractor for repeatability. Enable a real OCR or model adapter only after a separate quality and data-handling review. | More audit and evaluation data. Require replayable decisions, supported invoice rules agreed by the business, and a review route for unsupported or low-evidence input. |
| P6 · Stage AWS operations separately | Use independently deployable containers on ECS Fargate, private service discovery, managed PostgreSQL and narrowly scoped AWS task roles. Adopt infrastructure as code and one migration job per release. | Adds operational responsibility. Require staged rollout, isolation checks, restoration, queue replay and an approved budget before a customer cutover. |

## Inputs requiring an owner

| Resolve by | Decision needed |
|---|---|
| M1 design | Product/security owners choose identity provider or integration boundary, membership lifecycle, who may submit/edit/approve/override, and whether the same person may edit and approve. Define customer record and audit retention separately from session expiry. |
| Before private-document or provider use | Product and data owners agree on supported document types, invoice rules, allowed data regions and subprocessors, retention/deletion and review of parser/provider licensing. Choose OCR or a model against representative material and an approved budget. |
| Before staging and release | Operations owner agrees region, availability and recovery targets, expected peak workload, spending limits, alert ownership and rollout authority. Validate PostgreSQL and pgvector compatibility for the selected environment. |

These are decision points for the relevant milestones. The local tenant milestone can use a test identity issuer and two fictional organizations while business and operational owners finalize their production choices. [Migration gates](PRODUCTION_MIGRATION_PLAN.md)

## M1 implementation decisions · 25 November 2026

P1 and the local implementation of P2 are in effect. P3–P6 remain future work. The existing demo ties session and workspace IDs together. Migration 007 preserves those UUIDs while moving business foreign keys to tenants. The public sample contract and Redis/Python job envelope can therefore keep their existing fields. Child records either have matching composite keys or inherit ownership through their required parent.

Go uses operator-provisioned one-time sign-in for local named accounts. A separate provider and a new public enrollment surface were unnecessary for this milestone. The active membership comes from the issued code and is checked again at authorization time. The chosen role policy permits REVIEWER to approve clean invoices and reserves overrides for ADMIN and OWNER. A human may correct and later approve a clean invoice. New audit entries identify the applicable revision and ruleset. The retention choice preserves existing demo history after session cleanup and requires a future archival policy. [Role matrix and operational trade-offs](SECURITY.md)
