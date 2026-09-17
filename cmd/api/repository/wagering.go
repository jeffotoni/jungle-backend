package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend/internal/application/ports"
	"github.com/jeffotoni/jungle-backend/internal/domain/wager"
)

const wagerSelect = `
	SELECT id, provider_id, external_transaction_id, idempotency_key,
	       wallet_id, player_id, round_id, game_id, kind, amount, currency,
	       reference_external_transaction_id, reference_transaction_id,
	       status, payload_hash,
	       result_balance, failure_code, created_at, updated_at
	FROM wager_transactions`

func (s *Store) FindByID(ctx context.Context, tx pgx.Tx, providerID, id string) (ports.WagerRecord, error) {
	return s.scanWager(tx.QueryRow(ctx, wagerSelect+` WHERE id = $1 AND provider_id = $2`, mustUUID(id), providerID))
}

func (s *Store) FindByIdempotency(ctx context.Context, tx pgx.Tx, providerID, key string) (ports.WagerRecord, error) {
	return s.scanWager(tx.QueryRow(ctx, wagerSelect+` WHERE provider_id = $1 AND idempotency_key = $2`, providerID, key))
}

func (s *Store) FindByBusiness(
	ctx context.Context,
	tx pgx.Tx,
	providerID, externalID string,
) (ports.WagerRecord, error) {
	query := wagerSelect + ` WHERE provider_id = $1 AND external_transaction_id = $2`
	return s.scanWager(tx.QueryRow(ctx, query, providerID, externalID))
}

func (s *Store) FindReference(
	ctx context.Context,
	tx pgx.Tx,
	providerID, externalID string,
) (ports.WagerRecord, error) {
	return s.FindByBusiness(ctx, tx, providerID, externalID)
}

func (s *Store) FindReferenceForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	providerID, externalID string,
) (ports.WagerRecord, error) {
	query := wagerSelect + ` WHERE provider_id = $1 AND external_transaction_id = $2 FOR UPDATE`
	return s.scanWager(tx.QueryRow(ctx, query, providerID, externalID))
}

func (s *Store) HasSuccessfulReversal(
	ctx context.Context,
	tx pgx.Tx,
	providerID string,
	kind wager.Kind,
	reference string,
) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM wager_transactions
			WHERE provider_id = $1 AND kind = $2 AND
			      reference_external_transaction_id = $3 AND status = 'PROCESSED'
		)`, providerID, string(kind), reference).Scan(&exists)
	return exists, err
}

func (s *Store) InsertWager(ctx context.Context, tx pgx.Tx, record ports.WagerRecord) (bool, error) {
	id, err := parseUUID(record.ID)
	if err != nil {
		return false, err
	}
	referenceID, err := nullableUUID(record.ReferenceTransactionID)
	if err != nil {
		return false, err
	}
	var providerID, externalID, key any
	if record.ProviderID != nil {
		providerID = *record.ProviderID
	}
	if record.ExternalTransactionID != nil {
		externalID = *record.ExternalTransactionID
	}
	if record.IdempotencyKey != nil {
		key = *record.IdempotencyKey
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key,
			wallet_id, player_id, round_id, game_id, kind, amount, currency,
			reference_external_transaction_id, reference_transaction_id,
			status, payload_hash,
			result_balance, failure_code, occurred_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, now(), now(), now())
		ON CONFLICT DO NOTHING`,
		id,
		providerID,
		externalID,
		key,
		mustUUID(record.WalletID),
		record.PlayerID,
		nullIf(record.RoundID),
		nullIf(record.GameID),
		string(record.Kind),
		record.Amount,
		record.Currency,
		record.ReferenceExternalTransactionID,
		referenceID,
		string(record.Status),
		record.PayloadHash,
		record.ResultBalance,
		record.FailureCode,
	)
	if err != nil {
		return false, err
	}
	var inserted bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM wager_transactions WHERE id = $1)`, id).Scan(&inserted)
	return inserted, err
}

func (s *Store) UpdateWager(ctx context.Context, tx pgx.Tx, record ports.WagerRecord) error {
	referenceID, err := nullableUUID(record.ReferenceTransactionID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE wager_transactions
		SET reference_transaction_id = $2, status = $3, result_balance = $4,
		    failure_code = $5, updated_at = now()
		WHERE id = $1`, mustUUID(record.ID), referenceID,
		string(record.Status), record.ResultBalance, record.FailureCode)
	return err
}

func (s *Store) scanWager(row pgx.Row) (ports.WagerRecord, error) {
	var record ports.WagerRecord
	var id, wallet uuid.UUID
	var providerID, externalID, key, referenceExternalID, failureCode *string
	var referenceID *uuid.UUID
	var kind, status string
	if err := row.Scan(&id, &providerID, &externalID, &key, &wallet, &record.PlayerID, &record.RoundID,
		&record.GameID, &kind, &record.Amount, &record.Currency,
		&referenceExternalID, &referenceID, &status,
		&record.PayloadHash, &record.ResultBalance, &failureCode, &record.CreatedAt, &record.UpdatedAt); err != nil {
		return ports.WagerRecord{}, err
	}
	record.ID = id.String()
	record.WalletID = wallet.String()
	record.ProviderID = providerID
	record.ExternalTransactionID = externalID
	record.IdempotencyKey = key
	record.ReferenceExternalTransactionID = referenceExternalID
	if referenceID != nil {
		value := referenceID.String()
		record.ReferenceTransactionID = &value
	}
	record.FailureCode = failureCode
	record.Kind = wager.Kind(kind)
	record.Status = wager.Status(status)
	return record, nil
}

func nullIf(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableUUID(value *string) (any, error) {
	if value == nil {
		return nil, nil
	}
	return parseUUID(*value)
}
