# M1 identity, access and recovery

## Trust boundary

A signed-in browser has one active company. An operator verifies the person through a trusted channel and provisions a stable user UUID, an operator-controlled subject, a display name and a company membership. A person belonging to multiple companies signs in with a code for the intended membership. Names are display labels. Go derives the user, company and privileges from a database lookup. The HTTP API exposes no endpoint for issuing credentials or granting roles.

The operator issues a random 256-bit, single-use code valid for ten minutes. The browser sends it in a JSON body to the gateway over the configured origin. PostgreSQL atomically consumes the hashed code and creates a fresh eight-hour session. The cookie contains a new random secret. The database stores its digest. Cookies are host-only, HttpOnly, SameSite=Lax, scoped to `/api/v1` and Secure for HTTPS origins. Localhost HTTP is supported for development. The browser keeps the CSRF token in memory. Mutations require the exact origin and scoped mutations also require the matching CSRF header. Login attempts have a Redis peer quota. [Identity handlers](../services/gateway/identity.go) · [Session middleware](../services/gateway/main.go)

Each organization request checks membership status. Domain transactions lock the session and membership, re-read the role and execute under the restricted `nexus_request` database role with a transaction-local tenant setting. Revocation removes active sessions and outstanding codes for that membership. Reactivation requires a fresh sign-in. A decision that has already acquired its membership lock may commit before a concurrent revocation. After revocation commits the credential cannot authorize another decision. Signing out revokes the current session across browser tabs.

| Action | VIEWER | REVIEWER | ADMIN | OWNER |
|---|---|---|---|---|
| Read own company's invoices, jobs, history, sample previews and metrics | Yes | Yes | Yes | Yes |
| Ask about assigned policies | Yes | Yes | Yes | Yes |
| Submit, retry, correct or reject with a reason where required | No | Yes | Yes | Yes |
| Approve an invoice with clean checks | No | Yes | Yes | Yes |
| Approve with unresolved findings and a reason | No | No | Yes | Yes |

M1 gives OWNER and ADMIN the same invoice permissions. Membership administration uses the trusted operator tool. The policy allows a reviewer to correct an invoice and subsequently approve it once checks pass. A second independent approver is a separate business-policy decision. Cross-company IDs are concealed with HTTP 404 for an otherwise authorized reader/writer. Insufficient role returns 403. Missing, expired and revoked sessions return 401.

## Operate locally

From the repository root, start a local stack with Docker Compose. Migrations advance the selected database on startup. Use a backup and the upgrade procedure below when working with valuable data. In a private operator terminal define this helper. It uses the existing administrative seed container and passes the command arguments individually.

```sh
ops() {
  docker compose run --rm -T --no-deps \
    -v "$PWD/scripts/identity.py:/identity.py:ro" \
    seed python /identity.py "$@"
}
company_id=$(ops create-company "Fictional Company A")
reviewer_id=$(ops create-user "accounts/alex" "Alex Reviewer")
ops grant "$company_id" "$reviewer_id" REVIEWER
ops issue "$company_id" "$reviewer_id"
```

The last command prints the one-time credential to the operator. Deliver it directly to its verified recipient. Keep it out of command-line arguments, URLs, tickets, application logs and CI output. Open `http://localhost:3000/organization`, enter the code, process **Office supplies** and select **Approve as Alex Reviewer**. Sign out, issue a fresh code for the same membership and inspect the invoice's audit history. Save the returned UUIDs for later operations. To revoke this membership run `ops revoke "$company_id" "$reviewer_id"`. Other memberships for that user retain their own status. [Operator CLI](../scripts/identity.py)

## Storage and isolation

Migration 007 backfills one DEMO tenant per existing workspace using its original UUID. The historical column `invoices.workspace_id` is now a required foreign key to the durable tenant. Jobs carry an enforced matching tenant and invoice. Revisions and notification records inherit ownership through their invoice, and outbox entries through their job. Audit events carry a tenant and an optional actor-membership reference. New named actions store the display-name snapshot, stable actor ID, effective role, invoice version, revision number and ruleset. Migration 008 enforces matching membership and tenant on revision authors, restricts privileged function name resolution and removes the gateway’s access to login-code digests. Historical unknowns stay null. Application roles have no permission to update or delete audit or revision rows. [Tenant schema](../migrations/007_tenant_identity.sql) · [Identity constraints](../migrations/008_identity_constraints.sql)

An anonymous demo session owns access to a DEMO tenant. Reset and expiry remove that access. Cleanup can delete the session row while its tenant, invoices and history persist. Archived demo data is inaccessible to subsequent visitors. Companies have no demo session row. The old purger cannot reach their data. Only demo workspaces receive the automatic fictional-policy assignment. A company with no explicitly assigned policy receives an abstention.

SQL row policies protect domain reads/writes under the public request role and policy reader. Request-local scope ends with the transaction. Private Go workers and dispatchers use a trusted cross-company backend role to find jobs, then validate document ownership through the invoice and fence completion by job, attempt and lease. The worker cannot authorize an invoice decision through its callback API. Review notifications remain demo-only. The public preview serves the same allowlisted fictional files to authorized sessions. Customer document storage and preview authorization belong to M2.

## Upgrade and rollback

1. Back up PostgreSQL and verify restoration in a separate database. Check the six prior migration checksums and inventory invoices and audit rows by workspace. Schedule the transactional backfill and constraint validation when table locks are acceptable. Quiesce old mutations and drain old callbacks before switching writers. M1 uses a tenant advisory lock where the previous binary locked the workspace row.
2. Run the one-shot migration against the intended environment, then start the matching gateway, policy service, worker and frontend. Verify the eight-row migration ledger, original IDs, invoice counts, historic actors, and named test-tenant access. Run the authorization and demo regression suites against an isolated test project. A migration failure rolls back its transaction.
3. To pause company access, set `ORGANIZATION_ACCESS=disabled` in the gateway environment and recreate the gateway. Recreate all gateway replicas. Company login and existing company credentials stop authorizing requests. Demo sessions continue. Keep M1 workers/gateway for accepted company jobs until they drain. Re-enable the flag to resume.
4. Keep schemas 007–008 and every business/audit row. If the previous binary is required, restore the pre-upgrade backup into a separate demo database and route only demo traffic there. Keep the upgraded database and its named history intact. This avoids mixing previous lock behavior and policy-reader assumptions with the new schema. A backup taken before M1 omits later approvals. Rehearse the switch in an isolated environment and resume company traffic only with a compatible release.

## Remaining production decisions

The identity boundary supports local, trusted issuance. External SSO and MFA, secure delivery at scale, directory synchronization, and customer self-service administration require a separate integration. A future OIDC adapter should pin the expected issuer, validate signature, audience, expiry, state and nonce, bind issuer/subject to the existing user and issue the same server-side session after checking membership.

The gateway and its internal services remain trusted. Compromise of the backend database credential or shared internal service secret exceeds the public request-role boundary. Production still needs separate workload credentials, key rotation, secure service networking, encrypted storage/backups, recovery drills, an agreed retention/deletion policy for archived demo and company data, and capacity testing of serialized tenant writes. This milestone uses only fictional PDFs and the mock extractor.
