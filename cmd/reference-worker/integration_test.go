package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jeffotoni/log"

	appwager "github.com/jeffotoni/jungle-backend/internal/application/wagering"
	"github.com/jeffotoni/jungle-backend/internal/repository/postgres"
)

func TestIntegrationPendingReferenceExpires(t *testing.T) {
	if os.Getenv("JUNGLE_INTEGRATION") != "1" {
		t.Skip("set JUNGLE_INTEGRATION=1 to run integration tests")
	}
	ctx := context.Background()
	databaseURL := envValue("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable")
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}

	walletID := uuid.NewString()
	playerID := "reference-integration-player-" + walletID
	transactionID := uuid.NewString()
	externalID := "reference-integration-" + transactionID
	idempotencyKey := "reference-integration-key-" + transactionID
	if _, err := pool.Exec(ctx, `
		INSERT INTO wallets (id, player_id, balance, currency, version)
		VALUES ($1, $2, 10000, 'BRL', 1)`, walletID, playerID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM wager_transactions WHERE wallet_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)
	})
	if _, err := pool.Exec(ctx, `
		INSERT INTO wager_transactions (
			id, provider_id, external_transaction_id, idempotency_key,
			wallet_id, player_id, round_id, game_id, kind, amount, currency,
			reference_external_transaction_id, status, payload_hash, result_balance,
			reference_attempts, reference_next_attempt_at, reference_pending_at,
			occurred_at, created_at, updated_at
		) VALUES ($1, 'provider-a', $2, $3, $4, $5, 'round-1', 'game-1',
			'REFUND', 100, 'BRL', 'missing-reference', 'PENDING_REFERENCE',
			'hash', 10000, 0, now(), now() - interval '1 hour', now(), now(), now())`,
		transactionID, externalID, idempotencyKey, walletID, playerID); err != nil {
		t.Fatal(err)
	}

	store := postgres.NewStore(pool)
	tx := postgres.NewTxManager(pool)
	worker := &ReferenceWorker{
		tx:           tx,
		pending:      store,
		wagers:       appwager.NewService(tx, store, store),
		logger:       log.New(log.Config{Level: log.DEBUG, ServiceName: "reference-worker-test"}),
		batchSize:    1,
		referenceTTL: time.Minute,
		maxAttempts:  10,
		retryBase:    time.Millisecond,
		retryMax:     time.Millisecond,
	}

	count, err := worker.processBatch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("processed records=%d want=1", count)
	}

	var status, failureCode string
	if err := pool.QueryRow(ctx, `
		SELECT status, failure_code FROM wager_transactions WHERE id = $1`, transactionID).
		Scan(&status, &failureCode); err != nil {
		t.Fatal(err)
	}
	if status != "REJECTED" || failureCode != pendingReferenceFailureCode {
		t.Fatalf("status=%s failureCode=%s want REJECTED,%s", status, failureCode, pendingReferenceFailureCode)
	}
	var eventCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM outbox_events
		WHERE aggregate_id = $1 AND event_type = 'WagerTransactionRejected'`, walletID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("rejection events=%d want=1", eventCount)
	}
}

func envValue(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
