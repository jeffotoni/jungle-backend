package postgres

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend/internal/application/ports"
)

func (s *Store) CreateWallet(ctx context.Context, tx pgx.Tx, record ports.WalletRecord) error {
	id, err := parseUUID(record.ID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO wallets (id, player_id, balance, currency, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now(), now())`,
		id, record.PlayerID, record.Balance, record.Currency, record.Version)
	if isUnique(err) {
		return ports.ErrUniqueViolation
	}
	return err
}

func (s *Store) GetWallet(ctx context.Context, tx pgx.Tx, id string) (ports.WalletRecord, error) {
	return s.scanWallet(ctx, tx.QueryRow(ctx, `
		SELECT id, player_id, balance, currency, version, created_at, updated_at
		FROM wallets WHERE id = $1`, mustUUID(id)))
}

func (s *Store) LockWallet(ctx context.Context, tx pgx.Tx, id string) (ports.WalletRecord, error) {
	return s.scanWallet(ctx, tx.QueryRow(ctx, `
		SELECT id, player_id, balance, currency, version, created_at, updated_at
		FROM wallets WHERE id = $1 FOR UPDATE`, mustUUID(id)))
}

func (s *Store) scanWallet(_ context.Context, row pgx.Row) (ports.WalletRecord, error) {
	var record ports.WalletRecord
	var id uuid.UUID
	if err := row.Scan(
		&id,
		&record.PlayerID,
		&record.Balance,
		&record.Currency,
		&record.Version,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return ports.WalletRecord{}, err
	}
	record.ID = id.String()
	return record, nil
}

func (s *Store) UpdateWallet(ctx context.Context, tx pgx.Tx, id string, balance, version int64) error {
	result, err := tx.Exec(ctx, `
		UPDATE wallets SET balance = $2, version = $3, updated_at = now()
		WHERE id = $1 AND version = $3 - 1`, mustUUID(id), balance, version)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("wallet version conflict")
	}
	return nil
}

func mustUUID(value string) uuid.UUID {
	id, _ := uuid.Parse(value)
	return id
}

func (s *Store) ListLedger(ctx context.Context, walletID, cursor string, limit int) ([]ports.LedgerRecord, error) {
	id, err := parseUUID(walletID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	createdAt, entryID, hasCursor, err := decodeCursor(cursor)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT id, wallet_id, wager_transaction_id, direction, amount, currency,
		       balance_before, balance_after, created_at
		FROM ledger_entries WHERE wallet_id = $1`
	args := []any{id}
	if hasCursor {
		query += ` AND (created_at, id) < ($2, $3)`
		args = append(args, createdAt, entryID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]ports.LedgerRecord, 0, limit+1)
	for rows.Next() {
		var entry ports.LedgerRecord
		var id, wallet uuid.UUID
		var transaction *uuid.UUID
		if err := rows.Scan(
			&id,
			&wallet,
			&transaction,
			&entry.Direction,
			&entry.Amount,
			&entry.Currency,
			&entry.BalanceBefore,
			&entry.BalanceAfter,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		entry.ID = id.String()
		entry.WalletID = wallet.String()
		if transaction != nil {
			entry.TransactionID = transaction.String()
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func encodeCursor(entry ports.LedgerRecord) string {
	value := entry.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00") + "|" + entry.ID
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func EncodeCursor(entry ports.LedgerRecord) string { return encodeCursor(entry) }

func ValidateCursor(value string) error {
	_, _, _, err := decodeCursor(value)
	return err
}

func decodeCursor(value string) (time.Time, uuid.UUID, bool, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, uuid.Nil, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, uuid.Nil, false, fmt.Errorf("%w: encoding", ErrInvalidCursor)
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, false, fmt.Errorf("%w: format", ErrInvalidCursor)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.Nil, false, fmt.Errorf("%w: id", ErrInvalidCursor)
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, false, fmt.Errorf("%w: timestamp", ErrInvalidCursor)
	}
	return createdAt, id, true, nil
}
