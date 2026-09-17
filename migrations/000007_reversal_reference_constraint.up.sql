ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS wager_reference_fields_check,
    ADD CONSTRAINT wager_reference_fields_check CHECK (
        (kind IN ('REFUND', 'ROLLBACK')
            AND NULLIF(BTRIM(reference_external_transaction_id), '') IS NOT NULL)
        OR (kind = 'WIN'
            AND (reference_external_transaction_id IS NULL
                OR NULLIF(BTRIM(reference_external_transaction_id), '') IS NOT NULL))
        OR (kind IN ('BET', 'LOSS') AND reference_external_transaction_id IS NULL)
        OR (kind = 'OPENING' AND reference_external_transaction_id IS NULL)
    );
