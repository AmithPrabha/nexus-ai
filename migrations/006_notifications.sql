-- Existing records already represent a successfully logged local notification.
ALTER TABLE sandbox_notifications
 ADD COLUMN delivery_state text NOT NULL DEFAULT 'RECORDED' CHECK(delivery_state IN ('PENDING','RECORDED','FAILED')),
 ADD COLUMN attempts int NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 3),
 ADD COLUMN available_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX notification_due ON sandbox_notifications(available_at) WHERE delivery_state='PENDING';
CREATE TABLE sandbox_receipts (
 event_id uuid PRIMARY KEY REFERENCES sandbox_notifications ON DELETE CASCADE,
 recorded_at timestamptz NOT NULL DEFAULT now()
);
GRANT UPDATE(delivery_state,attempts,available_at) ON sandbox_notifications TO nexus_gateway;
GRANT SELECT,INSERT ON sandbox_receipts TO nexus_gateway;
