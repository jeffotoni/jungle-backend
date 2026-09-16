DROP INDEX IF EXISTS idx_outbox_claimable;

ALTER TABLE outbox_events
    DROP COLUMN IF EXISTS lease_until,
    DROP COLUMN IF EXISTS claim_token;
