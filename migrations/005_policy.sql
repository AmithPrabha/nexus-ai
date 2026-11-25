CREATE TABLE policies (
 id text PRIMARY KEY, title text NOT NULL, version text NOT NULL, source_hash text NOT NULL
);
CREATE TABLE policy_chunks (
 id uuid PRIMARY KEY, policy_id text NOT NULL REFERENCES policies ON DELETE CASCADE, page int NOT NULL CHECK(page>0),
 clause text NOT NULL, body text NOT NULL, topic text NOT NULL, embedding vector(96) NOT NULL, embedding_version text NOT NULL,
 search_vector tsvector GENERATED ALWAYS AS (to_tsvector('english',topic || ' ' || body)) STORED,
 UNIQUE(policy_id,clause)
);
CREATE INDEX chunks_fts ON policy_chunks USING gin(search_vector);
CREATE TABLE workspace_policies (
 workspace_id uuid REFERENCES workspaces ON DELETE CASCADE, policy_id text REFERENCES policies ON DELETE CASCADE,
 PRIMARY KEY(workspace_id,policy_id)
);
GRANT SELECT ON policies,policy_chunks,workspace_policies TO nexus_gateway,nexus_policy;
GRANT INSERT ON workspace_policies TO nexus_gateway;
CREATE FUNCTION grant_demo_policies() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO workspace_policies SELECT NEW.id,id FROM policies;
 RETURN NEW;
END $$;
CREATE TRIGGER demo_policy_access AFTER INSERT ON workspaces FOR EACH ROW EXECUTE FUNCTION grant_demo_policies();
