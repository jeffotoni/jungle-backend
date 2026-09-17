package wallet

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/wager"
)

type walletTxFake struct{}

func (walletTxFake) WithinTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return fn(nil)
}

type walletStoreFake struct {
	created []ports.WalletRecord
	ledger  []ports.LedgerRecord
}

func (s *walletStoreFake) CreateWallet(_ context.Context, _ pgx.Tx, record ports.WalletRecord) error {
	s.created = append(s.created, record)
	return nil
}

func (s *walletStoreFake) GetWallet(context.Context, pgx.Tx, string) (ports.WalletRecord, error) {
	return ports.WalletRecord{}, pgx.ErrNoRows
}

func (s *walletStoreFake) LockWallet(context.Context, pgx.Tx, string) (ports.WalletRecord, error) {
	return ports.WalletRecord{}, pgx.ErrNoRows
}

func (s *walletStoreFake) UpdateWallet(context.Context, pgx.Tx, string, int64, int64) error {
	return nil
}

func (s *walletStoreFake) ListLedger(context.Context, string, string, int) ([]ports.LedgerRecord, error) {
	return s.ledger, nil
}

func (s *walletStoreFake) LedgerBalance(context.Context, pgx.Tx, string) (int64, int64, error) {
	return 0, 0, nil
}

type walletWagerStoreFake struct {
	wagers []ports.WagerRecord
	ledger []ports.LedgerRecord
	outbox []string
}

func (s *walletWagerStoreFake) FindByID(context.Context, pgx.Tx, string, string) (ports.WagerRecord, error) {
	return ports.WagerRecord{}, pgx.ErrNoRows
}

func (s *walletWagerStoreFake) FindByIdempotency(context.Context, pgx.Tx, string, string) (ports.WagerRecord, error) {
	return ports.WagerRecord{}, pgx.ErrNoRows
}

func (s *walletWagerStoreFake) FindByBusiness(context.Context, pgx.Tx, string, string) (ports.WagerRecord, error) {
	return ports.WagerRecord{}, pgx.ErrNoRows
}

func (s *walletWagerStoreFake) FindReference(context.Context, pgx.Tx, string, string) (ports.WagerRecord, error) {
	return ports.WagerRecord{}, pgx.ErrNoRows
}

func (s *walletWagerStoreFake) FindReferenceForUpdate(context.Context, pgx.Tx, string, string) (ports.WagerRecord, error) {
	return ports.WagerRecord{}, pgx.ErrNoRows
}

func (s *walletWagerStoreFake) HasSuccessfulReversal(context.Context, pgx.Tx, string, wager.Kind, string) (bool, error) {
	return false, nil
}

func (s *walletWagerStoreFake) InsertWager(_ context.Context, _ pgx.Tx, record ports.WagerRecord) (bool, error) {
	s.wagers = append(s.wagers, record)
	return true, nil
}

func (s *walletWagerStoreFake) UpdateWager(context.Context, pgx.Tx, ports.WagerRecord) error {
	return nil
}

func (s *walletWagerStoreFake) InsertLedger(_ context.Context, _ pgx.Tx, record ports.LedgerRecord) error {
	s.ledger = append(s.ledger, record)
	return nil
}

func (s *walletWagerStoreFake) InsertOutbox(_ context.Context, _ pgx.Tx, _, _, _, eventType string, _ []byte) error {
	s.outbox = append(s.outbox, eventType)
	return nil
}

func TestCreateWalletCreatesOpeningAndCreditLedger(t *testing.T) {
	wallets := &walletStoreFake{}
	wagers := &walletWagerStoreFake{}
	service := NewService(walletTxFake{}, wallets, wagers)
	balance, err := money.New(2500, "BRL")
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Create(context.Background(), "player-1", balance)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != 1 || result.Balance.MinorUnits() != 2500 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(wallets.created) != 1 {
		t.Fatalf("wallet inserts=%d want=1", len(wallets.created))
	}
	if len(wagers.wagers) != 1 || wagers.wagers[0].Kind != "OPENING" {
		t.Fatalf("unexpected opening records: %+v", wagers.wagers)
	}
	if len(wagers.ledger) != 1 {
		t.Fatalf("ledger inserts=%d want=1", len(wagers.ledger))
	}
	entry := wagers.ledger[0]
	if entry.Direction != "CREDIT" || entry.Amount != 2500 || entry.BalanceBefore != 0 || entry.BalanceAfter != 2500 {
		t.Fatalf("unexpected opening ledger: %+v", entry)
	}
	if len(wagers.outbox) != 2 {
		t.Fatalf("outbox inserts=%d want=2", len(wagers.outbox))
	}
}
