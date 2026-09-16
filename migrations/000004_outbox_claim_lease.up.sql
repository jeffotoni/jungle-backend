ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS claim_token TEXT,
    ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_outbox_claimable
    ON outbox_events (status, next_attempt_at, lease_until, created_at, id);
