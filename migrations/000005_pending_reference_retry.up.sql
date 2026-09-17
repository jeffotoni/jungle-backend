ALTER TABLE wager_transactions
    ADD COLUMN IF NOT EXISTS reference_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS reference_next_attempt_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS reference_pending_at TIMESTAMPTZ NULL;

UPDATE wager_transactions
SET reference_pending_at = COALESCE(reference_pending_at, updated_at, created_at),
    reference_next_attempt_at = COALESCE(reference_next_attempt_at, now())
WHERE status = 'PENDING_REFERENCE';

CREATE INDEX IF NOT EXISTS idx_wager_pending_reference
    ON wager_transactions (reference_next_attempt_at, reference_pending_at, created_at, id)
    WHERE status = 'PENDING_REFERENCE';
