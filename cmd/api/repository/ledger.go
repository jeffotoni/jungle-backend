package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
)

func (s *Store) InsertLedger(ctx context.Context, tx pgx.Tx, record ports.LedgerRecord) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (
			id, wallet_id, wager_transaction_id, direction, amount, currency,
			balance_before, balance_after, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())`,
		mustUUID(record.ID), mustUUID(record.WalletID), mustUUID(record.TransactionID),
		record.Direction, record.Amount, record.Currency, record.BalanceBefore, record.BalanceAfter)
	return err
}

func (s *Store) InsertOutbox(
	ctx context.Context,
	tx pgx.Tx,
	aggregateType, aggregateID, eventType string,
	payload []byte,
) error {
	if !json.Valid(payload) {
		return errors.New("invalid outbox payload")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO outbox_events (
			id, aggregate_type, aggregate_id, event_type, payload, status,
			attempts, next_attempt_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5::jsonb, 'PENDING', 0, now(), now())`,
		uuid.New(), aggregateType, aggregateID, eventType, payload)
	return err
}

func (s *Store) LedgerBalance(ctx context.Context, tx pgx.Tx, walletID string) (int64, int64, error) {
	var balance, entries int64
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT balance_after FROM ledger_entries
			 WHERE wallet_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1),
			0
		), COUNT(*)
		FROM ledger_entries WHERE wallet_id = $1`, mustUUID(walletID)).Scan(&balance, &entries)
	return balance, entries, err
}
