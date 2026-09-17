ALTER TABLE wager_transactions
    DROP CONSTRAINT IF EXISTS wager_reference_fields_check,
    ADD CONSTRAINT wager_reference_fields_check CHECK (
        (kind IN ('WIN', 'REFUND', 'ROLLBACK') OR reference_external_transaction_id IS NULL)
        AND (reference_external_transaction_id IS NULL
            OR NULLIF(BTRIM(reference_external_transaction_id), '') IS NOT NULL)
    );
