-- Keep the existing scope UUIDs, including every historical demo invoice.
CREATE TABLE tenants (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 kind text NOT NULL CHECK (kind IN ('DEMO','COMPANY')),
 name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 160)
);
INSERT INTO tenants SELECT id,'DEMO','Demo workspace' FROM workspaces;

-- Only expiring demo sessions live in workspaces. A company has no row here.
CREATE FUNCTION create_demo_tenant() RETURNS trigger
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
BEGIN
 INSERT INTO public.tenants(id,kind,name) VALUES(NEW.id,'DEMO','Demo workspace');
 RETURN NEW;
END $$;
CREATE TRIGGER create_demo_tenant BEFORE INSERT ON workspaces
 FOR EACH ROW EXECUTE FUNCTION create_demo_tenant();
ALTER TABLE workspaces ADD FOREIGN KEY(id) REFERENCES tenants(id);
-- The existing workspace_id column represents the durable tenant. Keep its name
-- for compatibility with the current SQL, Python contract and Redis envelope.
ALTER TABLE invoices DROP CONSTRAINT invoices_workspace_id_fkey,
 ADD FOREIGN KEY(workspace_id) REFERENCES tenants(id), ADD UNIQUE(id,workspace_id);
ALTER TABLE idempotency_keys DROP CONSTRAINT idempotency_keys_workspace_id_fkey,
 ADD FOREIGN KEY(workspace_id) REFERENCES tenants(id);
ALTER TABLE ai_runs DROP CONSTRAINT ai_runs_workspace_id_fkey,
 ADD FOREIGN KEY(workspace_id) REFERENCES tenants(id);
ALTER TABLE workspace_policies DROP CONSTRAINT workspace_policies_workspace_id_fkey,
 ADD FOREIGN KEY(workspace_id) REFERENCES tenants(id);

CREATE TABLE users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 subject text NOT NULL UNIQUE CHECK(length(trim(subject)) BETWEEN 1 AND 240),
 display_name text NOT NULL CHECK(length(trim(display_name)) BETWEEN 1 AND 160)
);
CREATE TABLE tenant_memberships (
 tenant_id uuid REFERENCES tenants ON DELETE RESTRICT,
 user_id uuid REFERENCES users ON DELETE RESTRICT,
 role text NOT NULL CHECK (role IN ('OWNER','ADMIN','REVIEWER','VIEWER')),
 status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN ('ACTIVE','REVOKED')),
 PRIMARY KEY(tenant_id,user_id)
);
CREATE INDEX memberships_user ON tenant_memberships(user_id,tenant_id);
CREATE FUNCTION company_membership() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS (SELECT FROM tenants WHERE id=NEW.tenant_id AND kind='COMPANY') THEN
  RAISE EXCEPTION 'Membership requires a company tenant';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER company_membership BEFORE INSERT OR UPDATE ON tenant_memberships
 FOR EACH ROW EXECUTE FUNCTION company_membership();

CREATE TABLE login_codes (
 digest bytea PRIMARY KEY CHECK(octet_length(digest)=32),
 tenant_id uuid NOT NULL, user_id uuid NOT NULL,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '10 minutes',
 FOREIGN KEY(tenant_id,user_id) REFERENCES tenant_memberships
);
CREATE TABLE user_sessions (
 digest bytea PRIMARY KEY CHECK(octet_length(digest)=32),
 tenant_id uuid NOT NULL, user_id uuid NOT NULL,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '8 hours',
 FOREIGN KEY(tenant_id,user_id) REFERENCES tenant_memberships
);
CREATE INDEX sessions_membership ON user_sessions(tenant_id,user_id);
CREATE INDEX codes_membership ON login_codes(tenant_id,user_id);
CREATE INDEX sessions_expiry ON user_sessions(expires_at);
CREATE INDEX login_codes_expiry ON login_codes(expires_at);
CREATE FUNCTION revoke_membership_sessions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='REVOKED' THEN
  DELETE FROM login_codes WHERE tenant_id=NEW.tenant_id AND user_id=NEW.user_id;
  DELETE FROM user_sessions WHERE tenant_id=NEW.tenant_id AND user_id=NEW.user_id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER revoke_sessions AFTER UPDATE OF status ON tenant_memberships
 FOR EACH ROW EXECUTE FUNCTION revoke_membership_sessions();

-- One row lock orders revocation, code redemption and authenticated requests.
-- The gateway cannot grant membership, mint login codes or set its own role.
CREATE FUNCTION member_context(session_digest bytea)
 RETURNS TABLE(tenant_id uuid,user_id uuid,display_name text,tenant_name text,role text,expires_at timestamptz)
 LANGUAGE sql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
 SELECT t.id,u.id,u.display_name,t.name,m.role,s.expires_at FROM user_sessions s
 JOIN tenant_memberships m USING(tenant_id,user_id) JOIN tenants t ON t.id=m.tenant_id
 JOIN users u ON u.id=m.user_id WHERE s.digest=session_digest AND m.status='ACTIVE'
 AND s.expires_at>clock_timestamp() FOR SHARE OF m,s
$$;
CREATE FUNCTION redeem_login(code_digest bytea, session_digest bytea) RETURNS boolean
 LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog,public AS $$
DECLARE ticket login_codes%ROWTYPE;
BEGIN
 SELECT c.* INTO ticket FROM login_codes c JOIN tenant_memberships m USING(tenant_id,user_id)
 WHERE c.digest=code_digest AND m.status='ACTIVE' AND c.expires_at>clock_timestamp() FOR SHARE OF m;
 IF NOT FOUND THEN RETURN false; END IF;
 DELETE FROM login_codes WHERE digest=code_digest AND expires_at>clock_timestamp();
 IF NOT FOUND THEN RETURN false; END IF;
 INSERT INTO user_sessions(digest,tenant_id,user_id) VALUES(session_digest,ticket.tenant_id,ticket.user_id);
 RETURN true;
END $$;
REVOKE ALL ON FUNCTION member_context(bytea),redeem_login(bytea,bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION member_context(bytea),redeem_login(bytea,bytea) TO nexus_gateway;
GRANT SELECT(digest,expires_at),DELETE ON user_sessions,login_codes TO nexus_gateway;

-- Keep the name as an audit snapshot. Older anonymous events retain NULL user_id.
ALTER TABLE audit_events ADD COLUMN tenant_id uuid, ADD COLUMN actor_user_id uuid,
 ADD COLUMN invoice_version int, ADD COLUMN revision_no int, ADD COLUMN actor_role text,
 ADD COLUMN ruleset text;
UPDATE audit_events e SET tenant_id=i.workspace_id FROM invoices i WHERE i.id=e.invoice_id;
ALTER TABLE audit_events ALTER tenant_id SET NOT NULL,
 ADD FOREIGN KEY(invoice_id,tenant_id) REFERENCES invoices(id,workspace_id),
 ADD FOREIGN KEY(tenant_id,actor_user_id) REFERENCES tenant_memberships;
CREATE INDEX audit_tenant ON audit_events(tenant_id,invoice_id,id);
ALTER TABLE invoice_revisions ADD COLUMN actor_user_id uuid REFERENCES users;
CREATE VIEW active_tenants AS SELECT t.id FROM tenants t WHERE
 (current_user <> 'nexus_request' OR t.id=nullif(current_setting('nexus.tenant',true),'')::uuid) AND (t.kind='COMPANY'
 OR EXISTS (SELECT FROM workspaces w WHERE w.id=t.id AND w.expires_at>now()));
GRANT SELECT ON active_tenants,tenants TO nexus_gateway;

-- All public invoice handlers run as this role in a transaction with a scoped
-- setting. The gateway's private worker/dispatcher requires cross-tenant jobs.
DO $$ BEGIN
 IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='nexus_request') THEN CREATE ROLE nexus_request NOLOGIN; END IF;
END $$;
GRANT nexus_request TO nexus_gateway;
GRANT SELECT ON active_tenants,samples,policies,policy_chunks,workspace_policies,
 invoices,jobs,outbox,invoice_revisions,audit_events,idempotency_keys,ai_runs,sandbox_notifications TO nexus_request;
GRANT INSERT ON invoices,jobs,outbox,invoice_revisions,audit_events,idempotency_keys,ai_runs,sandbox_notifications TO nexus_request;
GRANT UPDATE(status,version,current_revision) ON invoices TO nexus_request;
GRANT USAGE ON SEQUENCE audit_events_id_seq TO nexus_request;
CREATE FUNCTION request_tenant() RETURNS uuid LANGUAGE sql STABLE AS $$
 SELECT nullif(current_setting('nexus.tenant',true),'')::uuid
$$;

-- Child ownership follows its required, immutable parent relationship.
DO $$
DECLARE item record;
BEGIN
 FOR item IN SELECT * FROM (VALUES
 ('tenants','id=request_tenant()'),
 ('policies','EXISTS(SELECT FROM workspace_policies w WHERE w.policy_id=policies.id AND w.workspace_id=request_tenant())'),
 ('policy_chunks','EXISTS(SELECT FROM workspace_policies w WHERE w.policy_id=policy_chunks.policy_id AND w.workspace_id=request_tenant())'),
 ('invoices','workspace_id=request_tenant()'),
 ('audit_events','tenant_id=request_tenant()'),
 ('jobs','EXISTS(SELECT FROM invoices i WHERE i.id=invoice_id AND i.workspace_id=request_tenant())'),
 ('invoice_revisions','EXISTS(SELECT FROM invoices i WHERE i.id=invoice_id AND i.workspace_id=request_tenant())'),
 ('sandbox_notifications','EXISTS(SELECT FROM invoices i WHERE i.id=invoice_id AND i.workspace_id=request_tenant())'),
 ('outbox','EXISTS(SELECT FROM jobs j WHERE j.id=job_id)'),
 ('ai_runs','workspace_id=request_tenant() AND (job_id IS NULL OR EXISTS(SELECT FROM jobs j WHERE j.id=job_id))'),
 ('idempotency_keys','workspace_id=request_tenant()'),
 ('workspace_policies','workspace_id=request_tenant()')
 ) AS rules(tbl,expr) LOOP
  EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY',item.tbl);
  EXECUTE format('CREATE POLICY scoped ON %I TO nexus_request USING (%s) WITH CHECK (%s)',item.tbl,item.expr,item.expr);
  EXECUTE format('CREATE POLICY background ON %I TO nexus_gateway USING (true) WITH CHECK (true)',item.tbl);
 END LOOP;
END $$;
CREATE POLICY assigned_policies ON workspace_policies FOR SELECT TO nexus_policy USING(workspace_id=request_tenant());

-- Allow older demo writers to complete an in-flight operation during an upgrade.
CREATE FUNCTION stamp_audit_scope() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 SELECT workspace_id,version,current_revision INTO STRICT NEW.tenant_id,NEW.invoice_version,NEW.revision_no
 FROM invoices WHERE id=NEW.invoice_id;
 IF NEW.action IN ('approve','reject') AND NEW.actor_user_id IS NULL AND
    EXISTS(SELECT FROM tenants WHERE id=NEW.tenant_id AND kind='COMPANY') THEN
  RAISE EXCEPTION 'Company decisions require a named member';
 END IF;
 RETURN NEW;
END $$;
GRANT SELECT ON tenants TO nexus_request;
CREATE TRIGGER stamp_audit_scope BEFORE INSERT ON audit_events
 FOR EACH ROW EXECUTE FUNCTION stamp_audit_scope();

CREATE POLICY policy_catalog_reader ON policies FOR SELECT TO nexus_policy USING
 (EXISTS(SELECT FROM workspace_policies w WHERE w.workspace_id=request_tenant() AND w.policy_id=policies.id));
CREATE POLICY chunk_reader ON policy_chunks FOR SELECT TO nexus_policy USING
 (EXISTS(SELECT FROM workspace_policies w WHERE w.workspace_id=request_tenant() AND w.policy_id=policy_chunks.policy_id));

-- Validate the two ownership paths on run telemetry, including worker writes.
ALTER TABLE jobs ADD COLUMN tenant_id uuid;
UPDATE jobs j SET tenant_id=i.workspace_id FROM invoices i WHERE i.id=j.invoice_id;
ALTER TABLE jobs ALTER tenant_id SET NOT NULL, ADD UNIQUE(id,tenant_id),
 ADD FOREIGN KEY(invoice_id,tenant_id) REFERENCES invoices(id,workspace_id);
CREATE FUNCTION stamp_job_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 SELECT workspace_id INTO STRICT NEW.tenant_id FROM invoices WHERE id=NEW.invoice_id;
 RETURN NEW;
END $$;
CREATE TRIGGER stamp_job_tenant BEFORE INSERT ON jobs FOR EACH ROW EXECUTE FUNCTION stamp_job_tenant();
ALTER TABLE ai_runs ADD FOREIGN KEY(job_id,workspace_id) REFERENCES jobs(id,tenant_id);
