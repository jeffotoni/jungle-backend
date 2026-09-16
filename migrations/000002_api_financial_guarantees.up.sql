ALTER TABLE wallets
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE wallets
    ADD CONSTRAINT wallets_version_positive CHECK (version >= 1);

ALTER TABLE wallets
    ADD CONSTRAINT wallets_currency_format_check CHECK (currency ~ '^[A-Z]{3}$');

CREATE UNIQUE INDEX IF NOT EXISTS wallets_player_currency_unique
    ON wallets (player_id, currency);

ALTER TABLE wager_transactions
    ALTER COLUMN provider_id DROP NOT NULL,
    ALTER COLUMN external_transaction_id DROP NOT NULL,
    ALTER COLUMN round_id DROP NOT NULL,
    ALTER COLUMN game_id DROP NOT NULL,
    ALTER COLUMN reference_transaction_id TYPE UUID
        USING NULLIF(reference_transaction_id, '')::uuid,
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT,
    ADD COLUMN IF NOT EXISTS reference_external_transaction_id TEXT,
    ADD COLUMN IF NOT EXISTS failure_code TEXT;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_reference_transaction_fk
    FOREIGN KEY (reference_transaction_id) REFERENCES wager_transactions(id);

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_kind_check CHECK (kind IN ('BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK', 'OPENING')),
    ADD CONSTRAINT wager_status_check CHECK (status IN ('PENDING', 'PENDING_REFERENCE', 'PROCESSED', 'REJECTED', 'FAILED')),
    ADD CONSTRAINT wager_source_fields_check CHECK (
        (kind = 'OPENING' AND provider_id IS NULL AND external_transaction_id IS NULL AND idempotency_key IS NULL) OR
        (kind <> 'OPENING' AND provider_id IS NOT NULL AND external_transaction_id IS NOT NULL AND idempotency_key IS NOT NULL)
    );

CREATE UNIQUE INDEX IF NOT EXISTS wager_provider_idempotency_unique
    ON wager_transactions (provider_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS wager_successful_reversal_unique
    ON wager_transactions (provider_id, kind, reference_external_transaction_id)
    WHERE status = 'PROCESSED' AND reference_external_transaction_id IS NOT NULL;

ALTER TABLE ledger_entries
    ALTER COLUMN entry_type DROP NOT NULL,
    ALTER COLUMN wager_transaction_id SET NOT NULL,
    ADD COLUMN IF NOT EXISTS direction TEXT NOT NULL DEFAULT 'CREDIT',
    ADD COLUMN IF NOT EXISTS balance_before BIGINT NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS ledger_wallet_transaction_unique
    ON ledger_entries (wallet_id, wager_transaction_id)
    WHERE wager_transaction_id IS NOT NULL;

ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_direction_check CHECK (direction IN ('DEBIT', 'CREDIT'));

ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_amount_positive CHECK (amount > 0),
    ADD CONSTRAINT ledger_balance_transition_check CHECK (
        (direction = 'DEBIT' AND balance_after = balance_before - amount) OR
        (direction = 'CREDIT' AND balance_after = balance_before + amount)
    );

CREATE OR REPLACE FUNCTION prevent_ledger_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ledger entries are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS ledger_entries_immutable ON ledger_entries;
CREATE TRIGGER ledger_entries_immutable
    BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION prevent_ledger_mutation();
