package wagering

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend-challenge/internal/application"
	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
	"github.com/jeffotoni/jungle-backend-challenge/internal/contracts"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/wager"
)

type Service struct {
	tx      ports.TxManager
	wallets ports.WalletStore
	wagers  ports.WagerStore
}

type Result struct {
	TransactionID string
	Status        wager.Status
	Balance       *money.Money
	FailureCode   string
	Replay        bool
}

func NewService(tx ports.TxManager, wallets ports.WalletStore, wagers ports.WagerStore) *Service {
	return &Service{tx: tx, wallets: wallets, wagers: wagers}
}

func (s *Service) Process(ctx context.Context, request contracts.WagerRequest) (Result, error) {
	request.ProviderID = strings.TrimSpace(request.ProviderID)
	request.ExternalTransactionID = strings.TrimSpace(request.ExternalTransactionID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	request.WalletID = strings.TrimSpace(request.WalletID)
	request.PlayerID = strings.TrimSpace(request.PlayerID)
	request.RoundID = strings.TrimSpace(request.RoundID)
	request.GameID = strings.TrimSpace(request.GameID)
	request.Kind = strings.ToUpper(strings.TrimSpace(request.Kind))
	request.Money.Amount = strings.TrimSpace(request.Money.Amount)
	request.Money.Currency = strings.ToUpper(strings.TrimSpace(request.Money.Currency))
	if request.ProviderID == "" ||
		request.ExternalTransactionID == "" ||
		request.IdempotencyKey == "" ||
		request.WalletID == "" ||
		request.PlayerID == "" ||
		request.RoundID == "" ||
		request.GameID == "" {
		return Result{}, application.ErrInvalid
	}
	if _, err := uuid.Parse(request.WalletID); err != nil {
		return Result{}, application.ErrInvalid
	}
	kind := wager.Kind(request.Kind)
	if kind != wager.KindBet &&
		kind != wager.KindWin &&
		kind != wager.KindLoss &&
		kind != wager.KindRefund &&
		kind != wager.KindRollback {
		return Result{}, application.ErrInvalid
	}
	amount, err := money.Parse(request.Money.Amount, request.Money.Currency)
	if err != nil ||
		amount.Amount < 0 ||
		(kind == wager.KindLoss && amount.Amount != 0) ||
		(kind != wager.KindLoss && amount.Amount == 0) {
		return Result{}, application.ErrInvalid
	}
	if (kind == wager.KindRefund || kind == wager.KindRollback) &&
		(request.ReferenceExternalTransactionID == nil ||
			strings.TrimSpace(*request.ReferenceExternalTransactionID) == "") {
		return Result{}, application.ErrInvalid
	}
	if request.ReferenceExternalTransactionID != nil {
		value := strings.TrimSpace(*request.ReferenceExternalTransactionID)
		request.ReferenceExternalTransactionID = &value
		if value == "" || (kind != wager.KindWin && kind != wager.KindRefund && kind != wager.KindRollback) {
			return Result{}, application.ErrInvalid
		}
	}
	hash, err := contracts.CanonicalHash(request)
	if err != nil {
		return Result{}, err
	}
	transactionID := uuid.NewString()
	providerID := request.ProviderID
	externalID := request.ExternalTransactionID
	idempotencyKey := request.IdempotencyKey
	base := ports.WagerRecord{
		ID:                             transactionID,
		ProviderID:                     &providerID,
		ExternalTransactionID:          &externalID,
		IdempotencyKey:                 &idempotencyKey,
		WalletID:                       request.WalletID,
		PlayerID:                       request.PlayerID,
		RoundID:                        request.RoundID,
		GameID:                         request.GameID,
		Kind:                           kind,
		Amount:                         amount.Amount,
		Currency:                       amount.Currency,
		ReferenceExternalTransactionID: request.ReferenceExternalTransactionID,
		Status:                         wager.StatusPending,
		PayloadHash:                    hash,
	}
	var result Result
	err = s.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		for _, found := range []func(context.Context, pgx.Tx) (ports.WagerRecord, error){
			func(ctx context.Context, tx pgx.Tx) (ports.WagerRecord, error) {
				return s.wagers.FindByIdempotency(ctx, tx, providerID, idempotencyKey)
			},
			func(ctx context.Context, tx pgx.Tx) (ports.WagerRecord, error) {
				return s.wagers.FindByBusiness(ctx, tx, providerID, externalID)
			},
		} {
			existing, findErr := found(ctx, tx)
			if findErr == nil {
				result, err = replayOrConflict(existing, hash)
				return err
			}
			if !errors.Is(findErr, pgx.ErrNoRows) {
				return findErr
			}
		}
		inserted, err := s.wagers.InsertWager(ctx, tx, base)
		if err != nil {
			return err
		}
		if !inserted {
			for _, found := range []func(context.Context, pgx.Tx) (ports.WagerRecord, error){
				func(ctx context.Context, tx pgx.Tx) (ports.WagerRecord, error) {
					return s.wagers.FindByIdempotency(ctx, tx, providerID, idempotencyKey)
				},
				func(ctx context.Context, tx pgx.Tx) (ports.WagerRecord, error) {
					return s.wagers.FindByBusiness(ctx, tx, providerID, externalID)
				},
			} {
				existing, findErr := found(ctx, tx)
				if findErr == nil {
					result, err = replayOrConflict(existing, hash)
					return err
				}
			}
			return application.ErrConflict
		}
		return s.apply(ctx, tx, base, amount, &result)
	})
	if errors.Is(err, ports.ErrUniqueViolation) {
		return Result{}, application.ErrConflict
	}
	return result, err
}

func (s *Service) Get(ctx context.Context, providerID, transactionID string) (Result, error) {
	var record ports.WagerRecord
	err := s.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		var err error
		record, err = s.wagers.FindByID(ctx, tx, providerID, transactionID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, application.ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	result, err := replayOrConflict(record, record.PayloadHash)
	result.Replay = false
	return result, err
}

func (s *Service) GetByExternal(ctx context.Context, providerID, externalID string) (Result, error) {
	var record ports.WagerRecord
	err := s.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		var err error
		record, err = s.wagers.FindByBusiness(ctx, tx, providerID, externalID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, application.ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	result, err := replayOrConflict(record, record.PayloadHash)
	result.Replay = false
	return result, err
}

func replayOrConflict(record ports.WagerRecord, hash string) (Result, error) {
	if record.PayloadHash != hash {
		return Result{}, application.ErrConflict
	}
	result := Result{TransactionID: record.ID, Status: record.Status, Replay: true}
	if record.ResultBalance != nil {
		balance, err := money.New(*record.ResultBalance, record.Currency)
		if err != nil {
			return Result{}, err
		}
		result.Balance = &balance
	}
	if record.FailureCode != nil {
		result.FailureCode = *record.FailureCode
	}
	return result, nil
}

func (s *Service) apply(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	amount money.Money,
	result *Result,
) error {
	wallet, err := s.wallets.LockWallet(ctx, tx, record.WalletID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrNotFound
		}
		return err
	}
	if wallet.PlayerID != record.PlayerID || wallet.Currency != amount.Currency {
		return s.reject(ctx, tx, record, wallet.Balance, wallet.Currency, "WALLET_MISMATCH", result)
	}
	current, err := money.New(wallet.Balance, wallet.Currency)
	if err != nil {
		return err
	}
	if record.Kind == wager.KindRefund || record.Kind == wager.KindRollback {
		reference, err := s.resolveReference(ctx, tx, record, amount)
		if err != nil {
			if errors.Is(err, application.ErrPending) {
				result.Balance = &current
				result.TransactionID, result.Status = record.ID, wager.StatusPendingReference
				if err := s.update(ctx, tx, record, wager.StatusPendingReference, &current, "REFERENCE_PENDING"); err != nil {
					return err
				}
				return s.outbox(
					ctx,
					tx,
					record,
					contracts.EventWagerTransactionPendingReference,
					contracts.WagerTransactionPendingReferenceData{
						TransactionID: record.ID,
						WalletID:      record.WalletID,
						Status:        string(wager.StatusPendingReference),
						FailureCode:   "REFERENCE_PENDING",
					},
				)
			}
			return s.reject(ctx, tx, record, wallet.Balance, wallet.Currency, failureCode(err), result)
		}
		record.ReferenceTransactionID = &reference.ID
	}
	if record.Kind == wager.KindWin && record.ReferenceExternalTransactionID != nil {
		reference, err := s.resolveReference(ctx, tx, record, amount)
		if err != nil {
			return s.reject(ctx, tx, record, wallet.Balance, wallet.Currency, failureCode(err), result)
		}
		record.ReferenceTransactionID = &reference.ID
	}

	change := int64(0)
	direction := ""
	switch record.Kind {
	case wager.KindBet:
		change, direction = -amount.Amount, "DEBIT"
	case wager.KindWin, wager.KindRefund:
		change, direction = amount.Amount, "CREDIT"
	case wager.KindRollback:
		reference, err := s.wagers.FindReference(ctx, tx, providerID(record), *record.ReferenceExternalTransactionID)
		if err != nil {
			return err
		}
		if reference.Kind == wager.KindBet {
			change, direction = amount.Amount, "CREDIT"
		} else {
			change, direction = -amount.Amount, "DEBIT"
		}
	case wager.KindLoss:
		result.Balance = &current
		result.TransactionID, result.Status = record.ID, wager.StatusProcessed
		if err := s.update(ctx, tx, record, wager.StatusProcessed, &current, ""); err != nil {
			return err
		}
		return s.outbox(
			ctx,
			tx,
			record,
			contracts.EventWagerTransactionProcessed,
			contracts.WagerTransactionProcessedData{
				TransactionID: record.ID,
				WalletID:      record.WalletID,
				Kind:          string(record.Kind),
				Status:        string(wager.StatusProcessed),
				Balance:       moneyPayload(current),
			},
		)
	}
	afterAmount, err := current.Add(money.Money{Amount: change, Currency: current.Currency})
	if err != nil || afterAmount.Amount < 0 {
		code := "INSUFFICIENT_BALANCE"
		if record.Kind == wager.KindRefund || record.Kind == wager.KindRollback {
			code = "REVERSAL_INSUFFICIENT_BALANCE"
		}
		return s.reject(ctx, tx, record, wallet.Balance, wallet.Currency, code, result)
	}
	after := afterAmount
	if err := s.wallets.UpdateWallet(ctx, tx, wallet.ID, after.Amount, wallet.Version+1); err != nil {
		return err
	}
	if err := s.wagers.InsertLedger(ctx, tx, ports.LedgerRecord{
		ID:            uuid.NewString(),
		WalletID:      record.WalletID,
		TransactionID: record.ID,
		Direction:     direction,
		Amount:        amount.Amount,
		Currency:      amount.Currency,
		BalanceBefore: current.Amount,
		BalanceAfter:  after.Amount,
	}); err != nil {
		return err
	}
	if err := s.update(ctx, tx, record, wager.StatusProcessed, &after, ""); err != nil {
		return err
	}
	result.TransactionID, result.Status, result.Balance = record.ID, wager.StatusProcessed, &after
	if err := s.outbox(
		ctx,
		tx,
		record,
		contracts.EventWagerTransactionProcessed,
		contracts.WagerTransactionProcessedData{
			TransactionID: record.ID,
			WalletID:      record.WalletID,
			Kind:          string(record.Kind),
			Status:        string(wager.StatusProcessed),
			Balance:       moneyPayload(after),
		},
	); err != nil {
		return err
	}
	return s.outbox(
		ctx,
		tx,
		record,
		contracts.EventWalletBalanceChanged,
		contracts.WalletBalanceChangedData{
			WalletID:      record.WalletID,
			TransactionID: record.ID,
			Direction:     direction,
			Money:         moneyPayload(amount),
			BalanceBefore: moneyPayload(current),
			BalanceAfter:  moneyPayload(after),
			WalletVersion: wallet.Version + 1,
		},
	)
}

func (s *Service) resolveReference(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	amount money.Money,
) (ports.WagerRecord, error) {
	reference, err := s.wagers.FindReferenceForUpdate(ctx, tx, providerID(record), *record.ReferenceExternalTransactionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.WagerRecord{}, application.ErrPending
		}
		return ports.WagerRecord{}, err
	}
	if reference.Status != wager.StatusProcessed {
		return ports.WagerRecord{}, errors.New("reference not processed")
	}
	if reference.PlayerID != record.PlayerID ||
		reference.WalletID != record.WalletID ||
		reference.Currency != record.Currency ||
		reference.RoundID != record.RoundID ||
		reference.Amount != amount.Amount {
		return ports.WagerRecord{}, errors.New("reference mismatch")
	}
	if record.Kind == wager.KindRefund && reference.Kind != wager.KindBet {
		return ports.WagerRecord{}, errors.New("refund reference must be bet")
	}
	if record.Kind == wager.KindWin {
		if reference.Kind != wager.KindBet {
			return ports.WagerRecord{}, errors.New("win reference must be bet")
		}
		return reference, nil
	}
	if record.Kind == wager.KindRollback &&
		reference.Kind != wager.KindBet &&
		reference.Kind != wager.KindWin &&
		reference.Kind != wager.KindRefund {
		return ports.WagerRecord{}, errors.New("invalid rollback reference")
	}
	exists, err := s.wagers.HasSuccessfulReversal(
		ctx,
		tx,
		providerID(record),
		record.Kind,
		*record.ReferenceExternalTransactionID,
	)
	if err != nil {
		return ports.WagerRecord{}, err
	}
	if exists {
		return ports.WagerRecord{}, errors.New("duplicate reversal")
	}
	return reference, nil
}

func (s *Service) reject(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	balance int64,
	currency, code string,
	result *Result,
) error {
	current, err := money.New(balance, currency)
	if err != nil {
		return err
	}
	if err := s.update(ctx, tx, record, wager.StatusRejected, &current, code); err != nil {
		return err
	}
	result.TransactionID = record.ID
	result.Status = wager.StatusRejected
	result.Balance = &current
	result.FailureCode = code
	return s.outbox(
		ctx,
		tx,
		record,
		contracts.EventWagerTransactionRejected,
		contracts.WagerTransactionRejectedData{
			TransactionID: record.ID,
			WalletID:      record.WalletID,
			Status:        string(wager.StatusRejected),
			FailureCode:   code,
			Balance:       moneyPayload(current),
		},
	)
}

func (s *Service) update(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	status wager.Status,
	balance *money.Money,
	failureCode string,
) error {
	var amount *int64
	if balance != nil {
		value := balance.Amount
		amount = &value
	}
	var code *string
	if failureCode != "" {
		code = &failureCode
	}
	record.Status, record.ResultBalance, record.FailureCode = status, amount, code
	return s.wagers.UpdateWager(ctx, tx, record)
}

func (s *Service) outbox(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	eventType contracts.EventType,
	data any,
) error {
	payload, err := contracts.MarshalEvent(eventType, record.WalletID, record.ID, 1, data)
	if err != nil {
		return err
	}
	return s.wagers.InsertOutbox(ctx, tx, "wallet", record.WalletID, string(eventType), payload)
}

func failureCode(err error) string {
	if errors.Is(err, application.ErrPending) {
		return "REFERENCE_PENDING"
	}
	if strings.Contains(err.Error(), "mismatch") {
		return "REFERENCE_MISMATCH"
	}
	if strings.Contains(err.Error(), "not processed") {
		return "REFERENCE_NOT_PROCESSED"
	}
	if strings.Contains(err.Error(), "duplicate") {
		return "DUPLICATE_REVERSAL"
	}
	return "INVALID_OPERATION"
}

func moneyPayload(value money.Money) contracts.MoneyInput {
	return contracts.MoneyInput{Amount: value.String(), Currency: value.Currency}
}

func providerID(record ports.WagerRecord) string {
	if record.ProviderID == nil {
		return ""
	}
	return *record.ProviderID
}
