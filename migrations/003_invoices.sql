CREATE TABLE invoices (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 workspace_id uuid NOT NULL REFERENCES workspaces ON DELETE CASCADE,
 sample_id text NOT NULL REFERENCES samples,
 status text NOT NULL CHECK (status IN ('QUEUED','PROCESSING','READY_FOR_APPROVAL','PENDING_REVIEW','FAILED','APPROVED','REJECTED')),
 version integer NOT NULL DEFAULT 1, current_revision integer NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX invoices_scope ON invoices(workspace_id,status,id);
CREATE TABLE jobs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), invoice_id uuid NOT NULL REFERENCES invoices ON DELETE CASCADE,
 status text NOT NULL DEFAULT 'QUEUED' CHECK(status IN ('QUEUED','PROCESSING','SUCCEEDED','FAILED')),
 attempts int NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 3),
 lease_token text, lease_until timestamptz, error_code text,
 next_attempt_at timestamptz NOT NULL DEFAULT now(), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_invoice ON jobs(invoice_id,created_at DESC);
CREATE TABLE outbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), job_id uuid NOT NULL UNIQUE REFERENCES jobs ON DELETE CASCADE,
 available_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX outbox_due ON outbox(available_at);
CREATE TABLE invoice_revisions (
 invoice_id uuid REFERENCES invoices ON DELETE CASCADE, revision_no integer NOT NULL,
 fields jsonb NOT NULL, evidence jsonb NOT NULL, warnings jsonb NOT NULL, validation_results jsonb NOT NULL,
 actor text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(invoice_id,revision_no)
);
CREATE TABLE audit_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, invoice_id uuid NOT NULL REFERENCES invoices ON DELETE CASCADE,
 actor text NOT NULL, action text NOT NULL, from_state text, to_state text NOT NULL,
 reason text NOT NULL, request_id uuid NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_invoice ON audit_events(invoice_id,id);
CREATE TABLE idempotency_keys (
 workspace_id uuid REFERENCES workspaces ON DELETE CASCADE, key text NOT NULL,
 request_hash text NOT NULL, response jsonb NOT NULL, PRIMARY KEY(workspace_id,key)
);
CREATE TABLE ai_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), workspace_id uuid NOT NULL REFERENCES workspaces ON DELETE CASCADE,
 job_id uuid REFERENCES jobs ON DELETE CASCADE, attempt integer, operation text NOT NULL, mode text NOT NULL CHECK(mode='mock'),
 latency_ms integer NOT NULL CHECK(latency_ms>=0), status text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(job_id,attempt)
);
CREATE TABLE sandbox_notifications (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), invoice_id uuid NOT NULL REFERENCES invoices ON DELETE CASCADE,
 version integer NOT NULL, reason_codes jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(invoice_id,version)
);
GRANT SELECT,INSERT,UPDATE ON invoices,jobs,outbox TO nexus_gateway;
GRANT DELETE ON outbox TO nexus_gateway;
GRANT SELECT,INSERT ON invoice_revisions,audit_events,idempotency_keys,ai_runs,sandbox_notifications TO nexus_gateway;
GRANT USAGE ON SEQUENCE audit_events_id_seq TO nexus_gateway;
