-- Resolve trusted tables before any temporary schema in privileged functions.
ALTER FUNCTION member_context(bytea) SET search_path TO pg_catalog,public,pg_temp;
ALTER FUNCTION redeem_login(bytea,bytea) SET search_path TO pg_catalog,public,pg_temp;
ALTER FUNCTION create_demo_tenant() SET search_path TO pg_catalog,public,pg_temp;
REVOKE EXECUTE ON FUNCTION create_demo_tenant() FROM PUBLIC;
-- Cleanup needs the expiry column. Redemption takes place inside redeem_login.
REVOKE SELECT(digest) ON login_codes FROM nexus_gateway;

-- Validate the revision author's membership and its invoice owner as one unit.
ALTER TABLE invoice_revisions ADD COLUMN tenant_id uuid;
UPDATE invoice_revisions r SET tenant_id=i.workspace_id FROM invoices i WHERE i.id=r.invoice_id;
ALTER TABLE invoice_revisions ALTER tenant_id SET NOT NULL,
 ADD FOREIGN KEY(invoice_id,tenant_id) REFERENCES invoices(id,workspace_id),
 ADD FOREIGN KEY(tenant_id,actor_user_id) REFERENCES tenant_memberships;
ALTER FUNCTION stamp_job_tenant() RENAME TO stamp_invoice_tenant;
CREATE TRIGGER stamp_revision_tenant BEFORE INSERT ON invoice_revisions
 FOR EACH ROW EXECUTE FUNCTION stamp_invoice_tenant();
