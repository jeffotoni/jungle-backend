package wallet

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend-challenge/internal/application"
	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
	"github.com/jeffotoni/jungle-backend-challenge/internal/contracts"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
	domainwallet "github.com/jeffotoni/jungle-backend-challenge/internal/domain/wallet"
)

type Service struct {
	tx      ports.TxManager
	wallets ports.WalletStore
	wagers  ports.WagerStore
}

type WalletResult struct {
	ID        string
	PlayerID  string
	Balance   money.Money
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type LedgerPage struct {
	Entries []ports.LedgerRecord
}

type Reconciliation struct {
	Wallet         WalletResult
	Calculated     money.Money
	Difference     money.Money
	Consistent     bool
	CheckedEntries int64
}

func NewService(tx ports.TxManager, wallets ports.WalletStore, wagers ports.WagerStore) *Service {
	return &Service{tx: tx, wallets: wallets, wagers: wagers}
}

func (s *Service) Create(ctx context.Context, playerID string, balance money.Money) (WalletResult, error) {
	if playerID == "" || balance.Amount < 0 {
		return WalletResult{}, application.ErrInvalid
	}
	id := uuid.NewString()
	aggregate, err := domainwallet.New(id, playerID, balance)
	if err != nil {
		return WalletResult{}, application.ErrInvalid
	}
	now := time.Now().UTC()
	result := WalletResult{
		ID:        id,
		PlayerID:  playerID,
		Balance:   aggregate.Balance,
		Version:   aggregate.Version,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err = s.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := s.wallets.CreateWallet(ctx, tx, ports.WalletRecord{
			ID:       id,
			PlayerID: playerID,
			Balance:  balance.Amount,
			Currency: balance.Currency,
			Version:  1,
		}); err != nil {
			if errors.Is(err, ports.ErrUniqueViolation) {
				return application.ErrConflict
			}
			return err
		}
		if balance.Amount == 0 {
			return nil
		}
		openingID := uuid.NewString()
		opening := ports.WagerRecord{
			ID:          openingID,
			WalletID:    id,
			PlayerID:    playerID,
			Kind:        "OPENING",
			Amount:      balance.Amount,
			Currency:    balance.Currency,
			Status:      "PROCESSED",
			PayloadHash: "internal",
		}
		if inserted, err := s.wagers.InsertWager(ctx, tx, opening); err != nil {
			return err
		} else if !inserted {
			return application.ErrConflict
		}
		if err := s.wagers.InsertLedger(ctx, tx, ports.LedgerRecord{
			ID:            uuid.NewString(),
			WalletID:      id,
			TransactionID: openingID,
			Direction:     "CREDIT",
			Amount:        balance.Amount,
			Currency:      balance.Currency,
			BalanceBefore: 0,
			BalanceAfter:  balance.Amount,
		}); err != nil {
			return err
		}
		processed, err := openingEvent(result, openingID)
		if err != nil {
			return err
		}
		if err := s.wagers.InsertOutbox(ctx, tx, "wallet", id, "WagerTransactionProcessed", processed); err != nil {
			return err
		}
		changed, err := balanceChangedEvent(
			result,
			openingID,
			"CREDIT",
			balance,
			money.Money{Currency: balance.Currency},
			balance,
		)
		if err != nil {
			return err
		}
		return s.wagers.InsertOutbox(ctx, tx, "wallet", id, "WalletBalanceChanged", changed)
	})
	if err != nil {
		if err == application.ErrConflict {
			return WalletResult{}, err
		}
		return WalletResult{}, err
	}
	return result, nil
}

func walletResult(record ports.WalletRecord) (WalletResult, error) {
	balance, err := money.New(record.Balance, record.Currency)
	if err != nil {
		return WalletResult{}, err
	}
	return WalletResult{
		ID:        record.ID,
		PlayerID:  record.PlayerID,
		Balance:   balance,
		Version:   record.Version,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}, nil
}

func (s *Service) Get(ctx context.Context, id string) (WalletResult, error) {
	var result WalletResult
	err := s.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		record, err := s.wallets.GetWallet(ctx, tx, id)
		if err != nil {
			return err
		}
		result, err = walletResult(record)
		return err
	})
	if err != nil {
		if isNoRows(err) {
			return WalletResult{}, application.ErrNotFound
		}
		return WalletResult{}, err
	}
	return result, nil
}

func (s *Service) Ledger(ctx context.Context, id, cursor string, limit int) (LedgerPage, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return LedgerPage{}, err
	}
	entries, err := s.wallets.ListLedger(ctx, id, cursor, limit)
	if err != nil {
		return LedgerPage{}, err
	}
	return LedgerPage{Entries: entries}, nil
}

func (s *Service) Reconcile(ctx context.Context, id string) (Reconciliation, error) {
	var result Reconciliation
	err := s.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		record, err := s.wallets.LockWallet(ctx, tx, id)
		if err != nil {
			return err
		}
		walletResultValue, err := walletResult(record)
		if err != nil {
			return err
		}
		calculatedAmount, entries, err := s.wallets.LedgerBalance(ctx, tx, id)
		if err != nil {
			return err
		}
		calculated, err := money.New(calculatedAmount, record.Currency)
		if err != nil {
			return err
		}
		difference, err := walletResultValue.Balance.Sub(calculated)
		if err != nil {
			return err
		}
		result = Reconciliation{
			Wallet:         walletResultValue,
			Calculated:     calculated,
			Difference:     difference,
			Consistent:     difference.Amount == 0,
			CheckedEntries: entries,
		}
		return nil
	})
	if err != nil {
		if isNoRows(err) {
			return Reconciliation{}, application.ErrNotFound
		}
		return Reconciliation{}, err
	}
	return result, nil
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func openingEvent(wallet WalletResult, transactionID string) ([]byte, error) {
	return contracts.MarshalEvent(
		contracts.EventWagerTransactionProcessed,
		wallet.ID,
		transactionID,
		1,
		contracts.WagerTransactionProcessedData{
			TransactionID: transactionID,
			WalletID:      wallet.ID,
			Kind:          "OPENING",
			Status:        "PROCESSED",
			Balance:       moneyPayload(wallet.Balance),
		},
	)
}

func balanceChangedEvent(
	wallet WalletResult,
	transactionID, direction string,
	movement, before, after money.Money,
) ([]byte, error) {
	return contracts.MarshalEvent(
		contracts.EventWalletBalanceChanged,
		wallet.ID,
		transactionID,
		wallet.Version,
		contracts.WalletBalanceChangedData{
			WalletID:      wallet.ID,
			TransactionID: transactionID,
			Direction:     direction,
			Money:         moneyPayload(movement),
			BalanceBefore: moneyPayload(before),
			BalanceAfter:  moneyPayload(after),
			WalletVersion: wallet.Version,
		},
	)
}

func moneyPayload(value money.Money) contracts.MoneyInput {
	return contracts.MoneyInput{Amount: value.String(), Currency: value.Currency}
}
