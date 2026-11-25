#!/bin/sh
set -eu
# Keep installation roles separate from the unprivileged runtime connections.
psql -X -q -v ON_ERROR_STOP=1 <<'SQL'
\getenv gateway_pass GATEWAY_PASSWORD
\getenv policy_pass POLICY_PASSWORD
BEGIN;
SELECT pg_advisory_xact_lock(615230);
SELECT format('CREATE ROLE nexus_gateway LOGIN PASSWORD %L', :'gateway_pass')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'nexus_gateway') \gexec
SELECT format('CREATE ROLE nexus_policy LOGIN PASSWORD %L', :'policy_pass')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'nexus_policy') \gexec
CREATE TABLE IF NOT EXISTS schema_migrations(version text PRIMARY KEY, digest text NOT NULL);
GRANT SELECT ON schema_migrations TO nexus_gateway;
COMMIT;
SQL
for file in /migrations/[0-9]*.sql; do
    version=$(basename "$file")
    digest=$(sha256sum "$file" | cut -d ' ' -f 1)
    psql -X -q -v ON_ERROR_STOP=1 -v version="$version" -v digest="$digest" -v file="$file" <<'SQL'
BEGIN;
SELECT pg_advisory_xact_lock(615230);
SELECT EXISTS (SELECT FROM schema_migrations WHERE version = :'version' AND digest <> :'digest') AS drift,
       NOT EXISTS (SELECT FROM schema_migrations WHERE version = :'version') AS pending \gset
\if :drift
    DO $$ BEGIN RAISE EXCEPTION 'Applied migration checksum changed'; END $$;
\endif
\if :pending
    \i :file
    INSERT INTO schema_migrations VALUES (:'version', :'digest');
\endif
COMMIT;
SQL
done
