package wagering

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend-challenge/internal/application"
	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
	"github.com/jeffotoni/jungle-backend-challenge/internal/contracts"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/wager"
)

const testWalletID = "516be6a5-8338-4560-a723-0fc1e6e6e801"

type wageringTxFake struct{}

func (wageringTxFake) WithinTx(ctx context.Context, fn func(pgx.Tx) error) error {
	return fn(nil)
}

type wageringStoreFake struct {
	wallets map[string]ports.WalletRecord
	wagers  []ports.WagerRecord
	ledger  []ports.LedgerRecord
	outbox  []string
}

func (s *wageringStoreFake) CreateWallet(context.Context, pgx.Tx, ports.WalletRecord) error {
	return nil
}

func (s *wageringStoreFake) GetWallet(_ context.Context, _ pgx.Tx, id string) (ports.WalletRecord, error) {
	record, ok := s.wallets[id]
	if !ok {
		return ports.WalletRecord{}, pgx.ErrNoRows
	}
	return record, nil
}

func (s *wageringStoreFake) LockWallet(ctx context.Context, tx pgx.Tx, id string) (ports.WalletRecord, error) {
	return s.GetWallet(ctx, tx, id)
}

func (s *wageringStoreFake) UpdateWallet(_ context.Context, _ pgx.Tx, id string, balance, version int64) error {
	record, ok := s.wallets[id]
	if !ok {
		return pgx.ErrNoRows
	}
	if record.Version != version-1 {
		return errors.New("wallet version conflict")
	}
	record.Balance = balance
	record.Version = version
	s.wallets[id] = record
	return nil
}

func (s *wageringStoreFake) ListLedger(context.Context, string, string, int) ([]ports.LedgerRecord, error) {
	return s.ledger, nil
}

func (s *wageringStoreFake) LedgerBalance(context.Context, pgx.Tx, string) (int64, int64, error) {
	return 0, 0, nil
}

func (s *wageringStoreFake) FindByID(_ context.Context, _ pgx.Tx, providerID, id string) (ports.WagerRecord, error) {
	for _, record := range s.wagers {
		if record.ID == id && providerIDOf(record) == providerID {
			return record, nil
		}
	}
	return ports.WagerRecord{}, pgx.ErrNoRows
}

func (s *wageringStoreFake) FindByIdempotency(_ context.Context, _ pgx.Tx, providerID, key string) (ports.WagerRecord, error) {
	for _, record := range s.wagers {
		if providerIDOf(record) == providerID && stringValue(record.IdempotencyKey) == key {
			return record, nil
		}
	}
	return ports.WagerRecord{}, pgx.ErrNoRows
}

func (s *wageringStoreFake) FindByBusiness(_ context.Context, _ pgx.Tx, providerID, externalID string) (ports.WagerRecord, error) {
	for _, record := range s.wagers {
		if providerIDOf(record) == providerID && stringValue(record.ExternalTransactionID) == externalID {
			return record, nil
		}
	}
	return ports.WagerRecord{}, pgx.ErrNoRows
}

func (s *wageringStoreFake) FindReference(ctx context.Context, tx pgx.Tx, providerID, externalID string) (ports.WagerRecord, error) {
	return s.FindByBusiness(ctx, tx, providerID, externalID)
}

func (s *wageringStoreFake) FindReferenceForUpdate(ctx context.Context, tx pgx.Tx, providerID, externalID string) (ports.WagerRecord, error) {
	return s.FindByBusiness(ctx, tx, providerID, externalID)
}

func (s *wageringStoreFake) HasSuccessfulReversal(_ context.Context, _ pgx.Tx, providerID string, kind wager.Kind, reference string) (bool, error) {
	for _, record := range s.wagers {
		if providerIDOf(record) == providerID &&
			record.Kind == kind &&
			stringValue(record.ReferenceExternalTransactionID) == reference &&
			record.Status == wager.StatusProcessed {
			return true, nil
		}
	}
	return false, nil
}

func (s *wageringStoreFake) InsertWager(_ context.Context, _ pgx.Tx, record ports.WagerRecord) (bool, error) {
	for _, existing := range s.wagers {
		if existing.ID == record.ID {
			return false, nil
		}
	}
	s.wagers = append(s.wagers, record)
	return true, nil
}

func (s *wageringStoreFake) UpdateWager(_ context.Context, _ pgx.Tx, record ports.WagerRecord) error {
	for index, existing := range s.wagers {
		if existing.ID == record.ID {
			s.wagers[index] = record
			return nil
		}
	}
	return pgx.ErrNoRows
}

func (s *wageringStoreFake) InsertLedger(_ context.Context, _ pgx.Tx, record ports.LedgerRecord) error {
	s.ledger = append(s.ledger, record)
	return nil
}

func (s *wageringStoreFake) InsertOutbox(_ context.Context, _ pgx.Tx, _, _, eventType string, _ []byte) error {
	s.outbox = append(s.outbox, eventType)
	return nil
}

func newWageringService(balance int64) (*Service, *wageringStoreFake) {
	store := &wageringStoreFake{
		wallets: map[string]ports.WalletRecord{
			testWalletID: {
				ID:       testWalletID,
				PlayerID: "player-001",
				Balance:  balance,
				Currency: "BRL",
				Version:  1,
			},
		},
	}
	return NewService(wageringTxFake{}, store, store), store
}

func testWager(kind, externalID, idempotencyKey, amount string) contracts.WagerRequest {
	return contracts.WagerRequest{
		ProviderID:            "provider-a",
		ExternalTransactionID: externalID,
		IdempotencyKey:        idempotencyKey,
		WalletID:              testWalletID,
		PlayerID:              "player-001",
		RoundID:               "round-001",
		GameID:                "game-001",
		Kind:                  kind,
		Money: contracts.MoneyInput{
			Amount:   amount,
			Currency: "BRL",
		},
	}
}

func TestProcessBETWithSufficientBalance(t *testing.T) {
	service, store := newWageringService(2500)

	result, err := service.Process(context.Background(), testWager("BET", "bet-001", "key-001", "10.00"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != wager.StatusProcessed || result.Balance == nil || result.Balance.MinorUnits() != 1500 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if store.wallets[testWalletID].Balance != 1500 || store.wallets[testWalletID].Version != 2 {
		t.Fatalf("unexpected wallet: %+v", store.wallets[testWalletID])
	}
	if len(store.ledger) != 1 || store.ledger[0].Direction != "DEBIT" {
		t.Fatalf("unexpected ledger: %+v", store.ledger)
	}
}

func TestProcessBETWithInsufficientBalance(t *testing.T) {
	service, store := newWageringService(500)

	result, err := service.Process(context.Background(), testWager("BET", "bet-002", "key-002", "10.00"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != wager.StatusRejected || result.FailureCode != "INSUFFICIENT_BALANCE" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if store.wallets[testWalletID].Balance != 500 || store.wallets[testWalletID].Version != 1 {
		t.Fatalf("wallet changed after rejection: %+v", store.wallets[testWalletID])
	}
	if len(store.ledger) != 0 {
		t.Fatalf("ledger entries=%d want=0", len(store.ledger))
	}
}

func TestProcessIdempotencyReplayAndConflict(t *testing.T) {
	service, store := newWageringService(2500)
	request := testWager("BET", "bet-003", "key-003", "10.00")

	first, err := service.Process(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := service.Process(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || replay.TransactionID != first.TransactionID || replay.Balance == nil || replay.Balance.MinorUnits() != 1500 {
		t.Fatalf("unexpected replay: %+v", replay)
	}
	if len(store.wagers) != 1 || len(store.ledger) != 1 || store.wallets[testWalletID].Balance != 1500 {
		t.Fatalf("replay changed financial state: wagers=%d ledger=%d wallet=%+v", len(store.wagers), len(store.ledger), store.wallets[testWalletID])
	}

	conflicting := request
	conflicting.Money.Amount = "11.00"
	if _, err := service.Process(context.Background(), conflicting); !errors.Is(err, application.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if len(store.ledger) != 1 || store.wallets[testWalletID].Balance != 1500 {
		t.Fatal("conflict changed financial state")
	}
}

func TestProcessBusinessIdentityConflictWithDifferentIdempotencyKey(t *testing.T) {
	service, store := newWageringService(2500)
	request := testWager("BET", "bet-004", "key-004", "10.00")

	if _, err := service.Process(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	conflicting := request
	conflicting.IdempotencyKey = "another-key"
	conflicting.Money.Amount = "11.00"
	if _, err := service.Process(context.Background(), conflicting); !errors.Is(err, application.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if len(store.ledger) != 1 || store.wallets[testWalletID].Balance != 1500 {
		t.Fatal("business conflict changed financial state")
	}
}

func TestProcessLOSSDoesNotChangeBalanceOrCreateLedger(t *testing.T) {
	service, store := newWageringService(2500)

	result, err := service.Process(context.Background(), testWager("LOSS", "loss-001", "key-loss-001", "0.00"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != wager.StatusProcessed || result.Balance == nil || result.Balance.MinorUnits() != 2500 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if store.wallets[testWalletID].Balance != 2500 || store.wallets[testWalletID].Version != 1 {
		t.Fatalf("wallet changed after LOSS: %+v", store.wallets[testWalletID])
	}
	if len(store.ledger) != 0 {
		t.Fatalf("ledger entries=%d want=0", len(store.ledger))
	}
	if len(store.outbox) != 1 || store.outbox[0] != "WagerTransactionProcessed" {
		t.Fatalf("unexpected LOSS events: %+v", store.outbox)
	}
}

func TestProcessREFUNDValidatesBETReference(t *testing.T) {
	service, store := newWageringService(2500)
	if _, err := service.Process(context.Background(), testWager("BET", "bet-refund", "key-bet-refund", "10.00")); err != nil {
		t.Fatal(err)
	}

	refund := testWager("REFUND", "refund-001", "key-refund-001", "10.00")
	refund.ReferenceExternalTransactionID = stringPointer("bet-refund")
	result, err := service.Process(context.Background(), refund)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != wager.StatusProcessed || result.Balance == nil || result.Balance.MinorUnits() != 2500 {
		t.Fatalf("unexpected refund: %+v", result)
	}
	if len(store.ledger) != 2 || store.ledger[1].Direction != "CREDIT" {
		t.Fatalf("unexpected refund ledger: %+v", store.ledger)
	}
	if store.wagers[1].ReferenceTransactionID == nil || *store.wagers[1].ReferenceTransactionID != store.wagers[0].ID {
		t.Fatalf("reference transaction was not persisted: %+v", store.wagers[1])
	}
}

func TestProcessROLLBACKValidatesWINReference(t *testing.T) {
	service, store := newWageringService(2500)
	if _, err := service.Process(context.Background(), testWager("WIN", "win-ref", "key-win-ref", "10.00")); err != nil {
		t.Fatal(err)
	}

	rollback := testWager("ROLLBACK", "rollback-001", "key-rollback-001", "10.00")
	rollback.ReferenceExternalTransactionID = stringPointer("win-ref")
	result, err := service.Process(context.Background(), rollback)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != wager.StatusProcessed || result.Balance == nil || result.Balance.MinorUnits() != 2500 {
		t.Fatalf("unexpected rollback: %+v", result)
	}
	if len(store.ledger) != 2 || store.ledger[1].Direction != "DEBIT" {
		t.Fatalf("unexpected rollback ledger: %+v", store.ledger)
	}
	if store.wagers[1].ReferenceTransactionID == nil || *store.wagers[1].ReferenceTransactionID != store.wagers[0].ID {
		t.Fatalf("reference transaction was not persisted: %+v", store.wagers[1])
	}
}

func TestProcessReversalRejectsReferenceMismatch(t *testing.T) {
	service, store := newWageringService(2500)
	if _, err := service.Process(context.Background(), testWager("BET", "bet-mismatch", "key-bet-mismatch", "10.00")); err != nil {
		t.Fatal(err)
	}

	refund := testWager("REFUND", "refund-mismatch", "key-refund-mismatch", "10.00")
	refund.RoundID = "another-round"
	refund.ReferenceExternalTransactionID = stringPointer("bet-mismatch")
	result, err := service.Process(context.Background(), refund)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != wager.StatusRejected || result.FailureCode != "REFERENCE_MISMATCH" {
		t.Fatalf("unexpected mismatch result: %+v", result)
	}
	if store.wallets[testWalletID].Balance != 1500 || len(store.ledger) != 1 {
		t.Fatalf("reference mismatch changed financial state: wallet=%+v ledger=%d", store.wallets[testWalletID], len(store.ledger))
	}
}

func providerIDOf(record ports.WagerRecord) string {
	return stringValue(record.ProviderID)
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func stringPointer(value string) *string {
	return &value
}
