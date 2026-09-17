DROP TRIGGER IF EXISTS wager_status_transition_guard ON wager_transactions;
DROP FUNCTION IF EXISTS prevent_invalid_wager_status_transition();

ALTER TABLE ledger_entries
    DROP CONSTRAINT IF EXISTS ledger_balance_non_negative_check,
    DROP CONSTRAINT IF EXISTS ledger_currency_brl_check;

ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS wager_failure_code_non_empty_check,
    DROP CONSTRAINT IF EXISTS wager_processed_reference_check,
    DROP CONSTRAINT IF EXISTS wager_reference_fields_check,
    DROP CONSTRAINT IF EXISTS wager_source_fields_check,
    DROP CONSTRAINT IF EXISTS wager_result_balance_non_negative_check,
    DROP CONSTRAINT IF EXISTS wager_amount_by_kind_check,
    DROP CONSTRAINT IF EXISTS wager_amount_non_negative_check,
    DROP CONSTRAINT IF EXISTS wager_currency_brl_check;

ALTER TABLE wager_transactions
    ADD CONSTRAINT wager_source_fields_check CHECK (
        (kind = 'OPENING' AND provider_id IS NULL AND external_transaction_id IS NULL AND idempotency_key IS NULL) OR
        (kind <> 'OPENING' AND provider_id IS NOT NULL AND external_transaction_id IS NOT NULL AND idempotency_key IS NOT NULL)
    );

ALTER TABLE wallets
    DROP CONSTRAINT IF EXISTS wallets_currency_brl_check,
    DROP CONSTRAINT IF EXISTS wallets_currency_format_check,
    ADD CONSTRAINT wallets_currency_format_check CHECK (currency ~ '^[A-Z]{3}$');
