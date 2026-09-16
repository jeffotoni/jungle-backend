DROP TRIGGER IF EXISTS ledger_entries_immutable ON ledger_entries;
DROP FUNCTION IF EXISTS prevent_ledger_mutation();
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_direction_check;
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_amount_positive;
ALTER TABLE ledger_entries DROP CONSTRAINT IF EXISTS ledger_balance_transition_check;
DROP INDEX IF EXISTS ledger_wallet_transaction_unique;
ALTER TABLE ledger_entries ALTER COLUMN wager_transaction_id DROP NOT NULL;
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS balance_before;
ALTER TABLE ledger_entries DROP COLUMN IF EXISTS direction;
ALTER TABLE ledger_entries ALTER COLUMN entry_type SET NOT NULL;
DROP INDEX IF EXISTS wager_successful_reversal_unique;
DROP INDEX IF EXISTS wager_provider_idempotency_unique;
ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS wager_reference_transaction_fk;
ALTER TABLE wager_transactions
    ALTER COLUMN reference_transaction_id TYPE TEXT
        USING reference_transaction_id::text;
ALTER TABLE wager_transactions DROP CONSTRAINT IF EXISTS wager_source_fields_check;
ALTER TABLE wager_transactions DROP CONSTRAINT IF EXISTS wager_status_check;
ALTER TABLE wager_transactions DROP CONSTRAINT IF EXISTS wager_kind_check;
ALTER TABLE wager_transactions DROP COLUMN IF EXISTS failure_code;
ALTER TABLE wager_transactions DROP COLUMN IF EXISTS reference_external_transaction_id;
ALTER TABLE wager_transactions DROP COLUMN IF EXISTS idempotency_key;
ALTER TABLE wager_transactions ALTER COLUMN game_id SET NOT NULL;
ALTER TABLE wager_transactions ALTER COLUMN round_id SET NOT NULL;
ALTER TABLE wager_transactions ALTER COLUMN external_transaction_id SET NOT NULL;
ALTER TABLE wager_transactions ALTER COLUMN provider_id SET NOT NULL;
DROP INDEX IF EXISTS wallets_player_currency_unique;
ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_version_positive;
ALTER TABLE wallets DROP CONSTRAINT IF EXISTS wallets_currency_iso_check;
ALTER TABLE wallets DROP COLUMN IF EXISTS version;
