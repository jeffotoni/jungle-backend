DROP INDEX IF EXISTS idx_wager_pending_reference;

ALTER TABLE wager_transactions
    DROP COLUMN IF EXISTS reference_pending_at,
    DROP COLUMN IF EXISTS reference_next_attempt_at,
    DROP COLUMN IF EXISTS reference_attempts;
