CREATE EXTENSION IF NOT EXISTS vector;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

CREATE TABLE workspaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    session_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '24 hours')
);
CREATE INDEX workspaces_expiry ON workspaces(expires_at);
GRANT SELECT, INSERT, DELETE ON workspaces TO nexus_gateway;

CREATE TABLE samples (
    sample_id text PRIMARY KEY,
    title text NOT NULL,
    description text NOT NULL
);
GRANT SELECT ON samples TO nexus_gateway;
