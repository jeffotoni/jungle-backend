DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM wallets WHERE currency <> 'BRL') THEN
        RAISE EXCEPTION 'wallets contains a non-BRL currency';
    END IF;
    IF EXISTS (SELECT 1 FROM wager_transactions WHERE currency <> 'BRL') THEN
        RAISE EXCEPTION 'wager_transactions contains a non-BRL currency';
    END IF;
    IF EXISTS (SELECT 1 FROM ledger_entries WHERE currency <> 'BRL') THEN
        RAISE EXCEPTION 'ledger_entries contains a non-BRL currency';
    END IF;
    IF EXISTS (SELECT 1 FROM wager_transactions WHERE amount < 0) THEN
        RAISE EXCEPTION 'wager_transactions contains a negative amount';
    END IF;
    IF EXISTS (
        SELECT 1 FROM wager_transactions
        WHERE (kind = 'LOSS' AND amount <> 0)
           OR (kind IN ('BET', 'WIN', 'REFUND', 'ROLLBACK', 'OPENING') AND amount <= 0)
    ) THEN
        RAISE EXCEPTION 'wager_transactions contains an amount incompatible with its kind';
    END IF;
    IF EXISTS (SELECT 1 FROM wager_transactions WHERE result_balance IS NOT NULL AND result_balance < 0) THEN
        RAISE EXCEPTION 'wager_transactions contains a negative result balance';
    END IF;
    IF EXISTS (SELECT 1 FROM ledger_entries WHERE balance_before < 0 OR balance_after < 0) THEN
        RAISE EXCEPTION 'ledger_entries contains a negative balance';
    END IF;
    IF EXISTS (
        SELECT 1 FROM wager_transactions
        WHERE kind = 'OPENING'
          AND (provider_id IS NOT NULL OR external_transaction_id IS NOT NULL OR idempotency_key IS NOT NULL
               OR round_id IS NOT NULL OR game_id IS NOT NULL
               OR reference_external_transaction_id IS NOT NULL OR reference_transaction_id IS NOT NULL
               OR failure_code IS NOT NULL)
    ) THEN
        RAISE EXCEPTION 'OPENING contains external fields';
    END IF;
    IF EXISTS (
        SELECT 1 FROM wager_transactions
        WHERE kind <> 'OPENING'
          AND (NULLIF(BTRIM(provider_id), '') IS NULL
               OR NULLIF(BTRIM(external_transaction_id), '') IS NULL
               OR NULLIF(BTRIM(idempotency_key), '') IS NULL
               OR NULLIF(BTRIM(round_id), '') IS NULL
               OR NULLIF(BTRIM(game_id), '') IS NULL
               OR NULLIF(BTRIM(payload_hash), '') IS NULL)
    ) THEN
        RAISE EXCEPTION 'external wager contains an empty required field';
    END IF;
    IF EXISTS (
        SELECT 1 FROM wager_transactions
        WHERE kind IN ('REFUND', 'ROLLBACK')
          AND NULLIF(BTRIM(reference_external_transaction_id), '') IS NULL
    ) THEN
        RAISE EXCEPTION 'reversal contains no external reference';
    END IF;
END
$$;

ALTER TABLE wallets
    DROP CONSTRAINT IF EXISTS wallets_currency_format_check,
    ADD CONSTRAINT wallets_currency_brl_check CHECK (currency = 'BRL');

ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS wager_source_fields_check,
    ADD CONSTRAINT wager_currency_brl_check CHECK (currency = 'BRL'),
    ADD CONSTRAINT wager_amount_non_negative_check CHECK (amount >= 0),
    ADD CONSTRAINT wager_amount_by_kind_check CHECK (
        (kind = 'LOSS' AND amount = 0) OR
        (kind IN ('BET', 'WIN', 'REFUND', 'ROLLBACK', 'OPENING') AND amount > 0)
    ),
    ADD CONSTRAINT wager_result_balance_non_negative_check CHECK (
        result_balance IS NULL OR result_balance >= 0
    ),
    ADD CONSTRAINT wager_source_fields_check CHECK (
        (kind = 'OPENING' AND provider_id IS NULL AND external_transaction_id IS NULL AND idempotency_key IS NULL
            AND round_id IS NULL AND game_id IS NULL
            AND reference_external_transaction_id IS NULL AND reference_transaction_id IS NULL
            AND failure_code IS NULL) OR
        (kind <> 'OPENING' AND NULLIF(BTRIM(provider_id), '') IS NOT NULL
            AND NULLIF(BTRIM(external_transaction_id), '') IS NOT NULL
            AND NULLIF(BTRIM(idempotency_key), '') IS NOT NULL
            AND NULLIF(BTRIM(round_id), '') IS NOT NULL
            AND NULLIF(BTRIM(game_id), '') IS NOT NULL
            AND NULLIF(BTRIM(payload_hash), '') IS NOT NULL)
    ),
    ADD CONSTRAINT wager_reference_fields_check CHECK (
        (kind IN ('WIN', 'REFUND', 'ROLLBACK') OR reference_external_transaction_id IS NULL)
        AND (reference_external_transaction_id IS NULL OR NULLIF(BTRIM(reference_external_transaction_id), '') IS NOT NULL)
    ),
    ADD CONSTRAINT wager_processed_reference_check CHECK (
        kind NOT IN ('REFUND', 'ROLLBACK') OR
        status <> 'PROCESSED' OR reference_transaction_id IS NOT NULL
    ),
    ADD CONSTRAINT wager_failure_code_non_empty_check CHECK (
        failure_code IS NULL OR NULLIF(BTRIM(failure_code), '') IS NOT NULL
    );

ALTER TABLE ledger_entries
    ADD CONSTRAINT ledger_currency_brl_check CHECK (currency = 'BRL'),
    ADD CONSTRAINT ledger_balance_non_negative_check CHECK (balance_before >= 0 AND balance_after >= 0);

CREATE OR REPLACE FUNCTION prevent_invalid_wager_status_transition() RETURNS trigger AS $$
BEGIN
    IF OLD.status = NEW.status THEN
        RETURN NEW;
    END IF;

    IF OLD.status = 'PENDING' AND NEW.status IN ('PROCESSED', 'PENDING_REFERENCE', 'REJECTED', 'FAILED') THEN
        RETURN NEW;
    END IF;
    IF OLD.status = 'PENDING_REFERENCE' AND NEW.status IN ('PROCESSED', 'REJECTED', 'FAILED') THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'invalid wager status transition: % -> %', OLD.status, NEW.status
        USING ERRCODE = '23514';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS wager_status_transition_guard ON wager_transactions;
CREATE TRIGGER wager_status_transition_guard
    BEFORE UPDATE OF status ON wager_transactions
    FOR EACH ROW EXECUTE FUNCTION prevent_invalid_wager_status_transition();
