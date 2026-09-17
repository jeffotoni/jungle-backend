package repository

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	appwager "github.com/jeffotoni/jungle-backend/internal/application/wagering"
	"github.com/jeffotoni/jungle-backend/internal/contracts"
	"github.com/jeffotoni/jungle-backend/internal/domain/wager"
	"github.com/jeffotoni/jungle-backend/internal/repository/postgres"
)

func TestPostgreSQLConcurrentBetsProtectWalletBalance(t *testing.T) {
	pool, ctx := integrationPool(t)
	walletID := uuid.NewString()
	playerID := "player-concurrent-" + walletID
	createIntegrationWallet(t, ctx, pool, walletID, playerID, 10000)
	cleanupIntegrationWallet(t, ctx, pool, walletID)

	service := appwager.NewService(
		postgres.NewTxManager(pool),
		NewStore(pool),
		NewStore(pool),
	)
	results := make(chan appwager.Result, 2)
	errorsCh := make(chan error, 2)
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	for index := 0; index < 2; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			result, err := service.Process(ctx, contracts.WagerRequest{
				ProviderID:            "provider-a",
				ExternalTransactionID: "concurrent-external-" + strconv.Itoa(index) + "-" + walletID,
				IdempotencyKey:        "concurrent-key-" + strconv.Itoa(index) + "-" + walletID,
				WalletID:              walletID,
				PlayerID:              playerID,
				RoundID:               "round-concurrent",
				GameID:                "game-concurrent",
				Kind:                  "BET",
				Money:                 contracts.MoneyInput{Amount: "80.00", Currency: "BRL"},
			})
			results <- result
			errorsCh <- err
		}(index)
	}
	close(start)
	waitGroup.Wait()
	close(results)
	close(errorsCh)

	processed := 0
	rejected := 0
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("unexpected processing error: %v", err)
		}
	}
	for result := range results {
		switch result.Status {
		case wager.StatusProcessed:
			processed++
		case wager.StatusRejected:
			rejected++
		default:
			t.Fatalf("unexpected result: %+v", result)
		}
	}
	if processed != 1 || rejected != 1 {
		t.Fatalf("processed=%d rejected=%d want one of each", processed, rejected)
	}

	var balance int64
	var version int64
	if err := pool.QueryRow(ctx, `SELECT balance, version FROM wallets WHERE id = $1`, walletID).Scan(&balance, &version); err != nil {
		t.Fatal(err)
	}
	if balance != 2000 || version != 2 {
		t.Fatalf("balance=%d version=%d want=2000,2", balance, version)
	}
	var debits int64
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM ledger_entries
		WHERE wallet_id = $1 AND direction = 'DEBIT'`, walletID).Scan(&debits); err != nil {
		t.Fatal(err)
	}
	if debits != 1 {
		t.Fatalf("debits=%d want=1", debits)
	}
}

func TestPostgreSQLTransactionRollsBackFinancialWrites(t *testing.T) {
	pool, ctx := integrationPool(t)
	walletID := uuid.NewString()
	wagerID := uuid.NewString()
	ledgerID := uuid.NewString()
	outboxID := uuid.NewString()
	txManager := postgres.NewTxManager(pool)

	err := txManager.WithinTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO wallets (id, player_id, balance, currency, version)
			VALUES ($1, $2, $3, $4, $5)`, walletID, "player-atomic", 1000, "BRL", 1); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO wager_transactions (
				id, wallet_id, player_id, kind, amount, currency, status,
				payload_hash, occurred_at
			) VALUES ($1, $2, $3, 'OPENING', $4, $5, 'PROCESSED', $6, now())`,
			wagerID, walletID, "player-atomic", 1000, "BRL", "atomicity"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (
				id, wallet_id, wager_transaction_id, direction, amount, currency,
				balance_before, balance_after
			) VALUES ($1, $2, $3, 'CREDIT', $4, $5, $6, $7)`,
			ledgerID, walletID, wagerID, 1000, "BRL", 0, 1000); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_events (
				id, aggregate_type, aggregate_id, event_type, payload
			) VALUES ($1, 'wallet', $2, 'WagerTransactionProcessed', '{}'::jsonb)`,
			outboxID, walletID); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil || err.Error() != "force rollback" {
		t.Fatalf("unexpected transaction error: %v", err)
	}

	var count int64
	for _, table := range []string{"wallets", "wager_transactions", "ledger_entries", "outbox_events"} {
		query := "SELECT COUNT(*) FROM " + table + " WHERE "
		column := "id"
		value := any(walletID)
		if table != "wallets" {
			column = "wallet_id"
		}
		if table == "outbox_events" {
			column = "aggregate_id"
		}
		if err := pool.QueryRow(ctx, query+column+" = $1", value).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("table=%s rows=%d want=0", table, count)
		}
	}
}

func integrationPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL unavailable: %v", err)
	}
	var table *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.wallets')`).Scan(&table); err != nil {
		t.Skipf("PostgreSQL schema unavailable: %v", err)
	}
	if table == nil {
		t.Skip("PostgreSQL migrations are not applied")
	}
	return pool, ctx
}

func createIntegrationWallet(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	walletID, playerID string,
	balance int64,
) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO wallets (id, player_id, balance, currency, version)
		VALUES ($1, $2, $3, 'BRL', 1)`, walletID, playerID, balance); err != nil {
		t.Fatal(err)
	}
}

func cleanupIntegrationWallet(t *testing.T, ctx context.Context, pool *pgxpool.Pool, walletID string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE aggregate_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_entries WHERE wallet_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM wager_transactions WHERE wallet_id = $1`, walletID)
		_, _ = pool.Exec(ctx, `DELETE FROM wallets WHERE id = $1`, walletID)
	})
}
