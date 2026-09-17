package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgreSQLFinancialConstraints(t *testing.T) {
	pool, ctx := integrationPool(t)
	if !financialConstraintsApplied(t, ctx, pool) {
		t.Skip("financial constraints migration is not applied")
	}

	walletID := uuid.NewString()
	playerID := "player-constraints-" + walletID
	if _, err := pool.Exec(ctx, `
		INSERT INTO wallets (id, player_id, balance, currency, version)
		VALUES ($1, $2, 10000, 'BRL', 1)`, walletID, playerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_entries WHERE wallet_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM wager_transactions WHERE wallet_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)
	})

	assertRejected(t, pool, ctx, `
		INSERT INTO wallets (id, player_id, balance, currency, version)
		VALUES ($1, $2, 0, 'USD', 1)`, uuid.NewString(), playerID+"-usd")

	assertRejected(t, pool, ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key,
			wallet_id, player_id, round_id, game_id, kind, amount, currency,
			status, payload_hash, occurred_at
		) VALUES ($1, 'provider-a', $2, $3, $4, $5, 'round-1', 'game-1', 'BET', -1, 'BRL', 'PENDING', 'hash', now())`,
		uuid.NewString(), "negative-"+walletID, "negative-key-"+walletID, walletID, playerID)

	assertRejected(t, pool, ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key,
			wallet_id, player_id, round_id, game_id, kind, amount, currency,
			status, payload_hash, occurred_at
		) VALUES ($1, 'provider-a', $2, $3, $4, $5, 'round-1', 'game-1', 'LOSS', 1, 'BRL', 'PENDING', 'hash', now())`,
		uuid.NewString(), "loss-"+walletID, "loss-key-"+walletID, walletID, playerID)

	assertRejected(t, pool, ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key,
			wallet_id, player_id, round_id, game_id, kind, amount, currency,
			status, payload_hash, occurred_at
		) VALUES ($1, '', $2, $3, $4, $5, 'round-1', 'game-1', 'BET', 100, 'BRL', 'PENDING', 'hash', now())`,
		uuid.NewString(), "empty-provider-"+walletID, "empty-provider-key-"+walletID, walletID, playerID)

	assertRejected(t, pool, ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key,
			wallet_id, player_id, round_id, game_id, kind, amount, currency,
			status, payload_hash, occurred_at
		) VALUES ($1, 'provider-a', $2, $3, $4, $5, 'round-1', 'game-1', 'REFUND', 100, 'BRL', 'PENDING', 'hash', now())`,
		uuid.NewString(), "refund-"+walletID, "refund-key-"+walletID, walletID, playerID)

	wagerID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key,
			wallet_id, player_id, round_id, game_id, kind, amount, currency,
			status, payload_hash, occurred_at
		) VALUES ($1, 'provider-a', $2, $3, $4, $5, 'round-1', 'game-1', 'BET', 100, 'BRL', 'PENDING', 'hash', now())`,
		wagerID, "transition-"+walletID, "transition-key-"+walletID, walletID, playerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE wager_transactions SET status = 'PROCESSED' WHERE id = $1`, wagerID); err != nil {
		t.Fatal(err)
	}
	assertRejected(t, pool, ctx, `UPDATE wager_transactions SET status = 'REJECTED' WHERE id = $1`, wagerID)

	assertRejected(t, pool, ctx, `
		INSERT INTO ledger_entries (
			id, wallet_id, wager_transaction_id, direction, amount, currency,
			balance_before, balance_after
		) VALUES ($1, $2, $3, 'DEBIT', 100, 'BRL', -1, -101)`,
		uuid.NewString(), walletID, wagerID)
}

func financialConstraintsApplied(t *testing.T, ctx context.Context, pool *pgxpool.Pool) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_constraint
			WHERE conrelid = 'wager_transactions'::regclass
			  AND conname = 'wager_amount_by_kind_check'
		)`)
	if err.Scan(&exists) != nil {
		return false
	}
	return exists
}

func assertRejected(t *testing.T, pool *pgxpool.Pool, ctx context.Context, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err == nil {
		t.Fatalf("expected PostgreSQL constraint rejection for query: %s", query)
	}
}
