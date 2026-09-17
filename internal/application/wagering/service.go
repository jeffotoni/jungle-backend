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
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/ledger"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/wager"
	domainwallet "github.com/jeffotoni/jungle-backend-challenge/internal/domain/wallet"
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
	request, base, err := prepareRequest(request)
	if err != nil {
		return Result{}, err
	}

	var result Result
	err = s.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		return s.processTx(ctx, tx, request, base, &result)
	})
	if errors.Is(err, ports.ErrUniqueViolation) {
		return Result{}, application.ErrConflict
	}
	var permanent *application.PermanentFailure
	if errors.As(err, &permanent) {
		return s.PersistPermanentFailure(ctx, request, permanent.Code)
	}
	return result, err
}

func (s *Service) ProcessInTx(
	ctx context.Context,
	tx pgx.Tx,
	request contracts.WagerRequest,
) (Result, error) {
	request, base, err := prepareRequest(request)
	if err != nil {
		return Result{}, err
	}
	var result Result
	err = s.processTx(ctx, tx, request, base, &result)
	if errors.Is(err, ports.ErrUniqueViolation) {
		return Result{}, application.ErrConflict
	}
	return result, err
}

func (s *Service) PersistPermanentFailure(
	ctx context.Context,
	request contracts.WagerRequest,
	code string,
) (Result, error) {
	request, base, err := prepareRequest(request)
	if err != nil {
		return Result{}, err
	}
	var result Result
	err = s.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		result, err = s.persistPermanentFailureInTx(ctx, tx, request, base, code)
		return err
	})
	return result, err
}

func (s *Service) PersistPermanentFailureInTx(
	ctx context.Context,
	tx pgx.Tx,
	request contracts.WagerRequest,
	code string,
) (Result, error) {
	request, base, err := prepareRequest(request)
	if err != nil {
		return Result{}, err
	}
	return s.persistPermanentFailureInTx(ctx, tx, request, base, code)
}

func (s *Service) persistPermanentFailureInTx(
	ctx context.Context,
	tx pgx.Tx,
	request contracts.WagerRequest,
	base ports.WagerRecord,
	code string,
) (Result, error) {
	if strings.TrimSpace(code) == "" {
		code = "PERMANENT_INFRASTRUCTURE_FAILURE"
	}
	for _, found := range []func(context.Context, pgx.Tx) (ports.WagerRecord, error){
		func(ctx context.Context, tx pgx.Tx) (ports.WagerRecord, error) {
			return s.wagers.FindByIdempotency(ctx, tx, request.ProviderID, request.IdempotencyKey)
		},
		func(ctx context.Context, tx pgx.Tx) (ports.WagerRecord, error) {
			return s.wagers.FindByBusiness(ctx, tx, request.ProviderID, request.ExternalTransactionID)
		},
	} {
		existing, findErr := found(ctx, tx)
		if findErr == nil {
			return replayOrConflict(existing, base.PayloadHash)
		}
		if !errors.Is(findErr, pgx.ErrNoRows) {
			return Result{}, findErr
		}
	}
	walletRecord, err := s.wallets.LockWallet(ctx, tx, request.WalletID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, application.ErrNotFound
		}
		return Result{}, err
	}
	balance, err := money.Rehydrate(walletRecord.Balance, walletRecord.Currency)
	if err != nil {
		return Result{}, err
	}
	transaction, err := rehydrateTransaction(base)
	if err != nil {
		return Result{}, err
	}
	if err := transaction.Fail(code, balance); err != nil {
		return Result{}, err
	}
	inserted, err := s.wagers.InsertWager(ctx, tx, wagerRecordFromTransaction(transaction))
	if err != nil {
		return Result{}, err
	}
	if !inserted {
		for _, found := range []func(context.Context, pgx.Tx) (ports.WagerRecord, error){
			func(ctx context.Context, tx pgx.Tx) (ports.WagerRecord, error) {
				return s.wagers.FindByIdempotency(ctx, tx, request.ProviderID, request.IdempotencyKey)
			},
			func(ctx context.Context, tx pgx.Tx) (ports.WagerRecord, error) {
				return s.wagers.FindByBusiness(ctx, tx, request.ProviderID, request.ExternalTransactionID)
			},
		} {
			existing, findErr := found(ctx, tx)
			if findErr == nil {
				return replayOrConflict(existing, base.PayloadHash)
			}
		}
		return Result{}, application.ErrConflict
	}
	return resultFromTransaction(transaction), nil
}

func prepareRequest(request contracts.WagerRequest) (contracts.WagerRequest, ports.WagerRecord, error) {
	normalized, err := contracts.NormalizeWagerRequest(request)
	if err != nil {
		return contracts.WagerRequest{}, ports.WagerRecord{}, application.ErrInvalid
	}
	request = normalized
	if request.ProviderID == "" ||
		request.ExternalTransactionID == "" ||
		request.IdempotencyKey == "" ||
		request.WalletID == "" ||
		request.PlayerID == "" ||
		request.RoundID == "" ||
		request.GameID == "" {
		return contracts.WagerRequest{}, ports.WagerRecord{}, application.ErrInvalid
	}
	if _, err := uuid.Parse(request.WalletID); err != nil {
		return contracts.WagerRequest{}, ports.WagerRecord{}, application.ErrInvalid
	}
	kind := wager.Kind(request.Kind)
	if kind != wager.KindBet &&
		kind != wager.KindWin &&
		kind != wager.KindLoss &&
		kind != wager.KindRefund &&
		kind != wager.KindRollback {
		return contracts.WagerRequest{}, ports.WagerRecord{}, application.ErrInvalid
	}
	amount, err := money.Parse(request.Money.Amount, request.Money.Currency)
	if err != nil ||
		amount.MinorUnits() < 0 ||
		(kind == wager.KindLoss && amount.MinorUnits() != 0) ||
		(kind != wager.KindLoss && amount.MinorUnits() == 0) {
		return contracts.WagerRequest{}, ports.WagerRecord{}, application.ErrInvalid
	}
	if (kind == wager.KindRefund || kind == wager.KindRollback) &&
		(request.ReferenceExternalTransactionID == nil ||
			*request.ReferenceExternalTransactionID == "") {
		return contracts.WagerRequest{}, ports.WagerRecord{}, application.ErrInvalid
	}
	if request.ReferenceExternalTransactionID != nil {
		if *request.ReferenceExternalTransactionID == "" ||
			(kind != wager.KindWin && kind != wager.KindRefund && kind != wager.KindRollback) {
			return contracts.WagerRequest{}, ports.WagerRecord{}, application.ErrInvalid
		}
	}
	hash, err := contracts.CanonicalHash(request)
	if err != nil {
		return contracts.WagerRequest{}, ports.WagerRecord{}, err
	}
	transactionID := uuid.NewString()
	providerID := request.ProviderID
	externalID := request.ExternalTransactionID
	idempotencyKey := request.IdempotencyKey
	transaction, err := wager.New(wager.Input{
		ID:                             transactionID,
		ProviderID:                     &providerID,
		ExternalTransactionID:          &externalID,
		IdempotencyKey:                 &idempotencyKey,
		WalletID:                       request.WalletID,
		PlayerID:                       request.PlayerID,
		RoundID:                        request.RoundID,
		GameID:                         request.GameID,
		Kind:                           kind,
		Money:                          amount,
		ReferenceExternalTransactionID: request.ReferenceExternalTransactionID,
		Status:                         wager.StatusPending,
		PayloadHash:                    hash,
	})
	if err != nil {
		return contracts.WagerRequest{}, ports.WagerRecord{}, application.ErrInvalid
	}
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
		Amount:                         transaction.Money().MinorUnits(),
		Currency:                       transaction.Money().Currency(),
		ReferenceExternalTransactionID: request.ReferenceExternalTransactionID,
		Status:                         wager.StatusPending,
		PayloadHash:                    hash,
	}
	return request, base, nil
}

func (s *Service) processTx(
	ctx context.Context,
	tx pgx.Tx,
	request contracts.WagerRequest,
	base ports.WagerRecord,
	result *Result,
) error {
	providerID := request.ProviderID
	externalID := request.ExternalTransactionID
	idempotencyKey := request.IdempotencyKey
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
			var err error
			*result, err = replayOrConflict(existing, base.PayloadHash)
			return err
		}
		if !errors.Is(findErr, pgx.ErrNoRows) {
			return findErr
		}
	}
	if _, lockErr := s.wallets.LockWallet(ctx, tx, request.WalletID); lockErr != nil {
		if errors.Is(lockErr, pgx.ErrNoRows) {
			return application.ErrNotFound
		}
		return lockErr
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
				var err error
				*result, err = replayOrConflict(existing, base.PayloadHash)
				return err
			}
		}
		return application.ErrConflict
	}
	return s.apply(ctx, tx, base, result)
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

func rehydrateTransaction(record ports.WagerRecord) (wager.Transaction, error) {
	value, err := money.Rehydrate(record.Amount, record.Currency)
	if err != nil {
		return wager.Transaction{}, err
	}
	var resultBalance *money.Money
	if record.ResultBalance != nil {
		result, err := money.Rehydrate(*record.ResultBalance, record.Currency)
		if err != nil {
			return wager.Transaction{}, err
		}
		resultBalance = &result
	}
	return wager.Rehydrate(wager.Input{
		ID:                             record.ID,
		ProviderID:                     record.ProviderID,
		ExternalTransactionID:          record.ExternalTransactionID,
		IdempotencyKey:                 record.IdempotencyKey,
		WalletID:                       record.WalletID,
		PlayerID:                       record.PlayerID,
		RoundID:                        record.RoundID,
		GameID:                         record.GameID,
		Kind:                           record.Kind,
		Money:                          value,
		ReferenceExternalTransactionID: record.ReferenceExternalTransactionID,
		ReferenceTransactionID:         record.ReferenceTransactionID,
		Status:                         record.Status,
		PayloadHash:                    record.PayloadHash,
		ResultBalance:                  resultBalance,
		FailureCode:                    record.FailureCode,
		OccurredAt:                     record.CreatedAt,
	})
}

func (s *Service) ResumePendingReference(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
) (Result, error) {
	if record.Status != wager.StatusPendingReference {
		return Result{}, application.ErrInvalid
	}
	transaction, err := rehydrateTransaction(record)
	if err != nil {
		return Result{}, err
	}
	var result Result
	err = s.applyWithPendingEvent(ctx, tx, record, transaction, &result, false)
	return result, err
}

func (s *Service) RejectPendingReference(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	code string,
) (Result, error) {
	wallet, err := s.wallets.LockWallet(ctx, tx, record.WalletID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Result{}, application.ErrNotFound
		}
		return Result{}, err
	}
	transaction, err := rehydrateTransaction(record)
	if err != nil {
		return Result{}, err
	}
	balance, err := money.Rehydrate(wallet.Balance, wallet.Currency)
	if err != nil {
		return Result{}, err
	}
	if err := transaction.Reject(code, balance); err != nil {
		return Result{}, err
	}
	var result Result
	err = s.persistTransaction(ctx, tx, transaction)
	if err == nil {
		result = resultFromTransaction(transaction)
		err = s.outbox(
			ctx,
			tx,
			transaction,
			contracts.EventWagerTransactionRejected,
			contracts.WagerTransactionRejectedData{
				TransactionID: transaction.ID(),
				WalletID:      transaction.WalletID(),
				Status:        string(transaction.Status()),
				FailureCode:   code,
				Balance:       moneyPayload(balance),
			},
		)
	}
	return result, err
}

func (s *Service) apply(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	result *Result,
) error {
	transaction, err := rehydrateTransaction(record)
	if err != nil {
		return err
	}
	return s.applyWithPendingEvent(ctx, tx, record, transaction, result, true)
}

func (s *Service) applyWithPendingEvent(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	transaction wager.Transaction,
	result *Result,
	emitPendingEvent bool,
) error {
	walletRecord, err := s.wallets.LockWallet(ctx, tx, record.WalletID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return application.ErrNotFound
		}
		return err
	}
	current, err := money.Rehydrate(walletRecord.Balance, walletRecord.Currency)
	if err != nil {
		return err
	}
	wallet, err := domainwallet.Rehydrate(walletRecord.ID, walletRecord.PlayerID, current, walletRecord.Version)
	if err != nil {
		return err
	}
	if wallet.PlayerID() != transaction.PlayerID() || wallet.Balance().Currency() != transaction.Money().Currency() {
		return s.rejectTransaction(ctx, tx, transaction, wallet.Balance(), "WALLET_MISMATCH", result)
	}

	var referenceKind *wager.Kind
	if transaction.Kind() == wager.KindRefund || transaction.Kind() == wager.KindRollback {
		reference, err := s.resolveReference(ctx, tx, record, transaction)
		if err != nil {
			if errors.Is(err, application.ErrPending) {
				if err := transaction.MarkPendingReference(wallet.Balance()); err != nil {
					return err
				}
				if err := s.persistTransaction(ctx, tx, transaction); err != nil {
					return err
				}
				*result = resultFromTransaction(transaction)
				if !emitPendingEvent {
					return nil
				}
				return s.outbox(
					ctx,
					tx,
					transaction,
					contracts.EventWagerTransactionPendingReference,
					contracts.WagerTransactionPendingReferenceData{
						TransactionID: transaction.ID(),
						WalletID:      transaction.WalletID(),
						Status:        string(wager.StatusPendingReference),
						FailureCode:   "REFERENCE_PENDING",
					},
				)
			}
			return s.rejectTransaction(ctx, tx, transaction, wallet.Balance(), failureCode(err), result)
		}
		if err := transaction.ResolveReference(reference.ID); err != nil {
			return err
		}
		kind := reference.Kind
		referenceKind = &kind
	}
	if transaction.Kind() == wager.KindWin && transaction.ReferenceExternalTransactionID() != nil {
		reference, err := s.resolveReference(ctx, tx, record, transaction)
		if err != nil {
			return s.rejectTransaction(ctx, tx, transaction, wallet.Balance(), failureCode(err), result)
		}
		if err := transaction.ResolveReference(reference.ID); err != nil {
			return err
		}
		kind := reference.Kind
		referenceKind = &kind
	}

	effect, err := transaction.FinancialEffect(referenceKind)
	if err != nil {
		return s.rejectTransaction(ctx, tx, transaction, wallet.Balance(), failureCode(err), result)
	}
	if !effect.Applied {
		if err := transaction.MarkProcessed(wallet.Balance()); err != nil {
			return err
		}
		if err := s.persistTransaction(ctx, tx, transaction); err != nil {
			return err
		}
		*result = resultFromTransaction(transaction)
		return s.outbox(
			ctx,
			tx,
			transaction,
			contracts.EventWagerTransactionProcessed,
			contracts.WagerTransactionProcessedData{
				TransactionID: transaction.ID(),
				WalletID:      transaction.WalletID(),
				Kind:          string(transaction.Kind()),
				Status:        string(transaction.Status()),
				Balance:       moneyPayload(wallet.Balance()),
			},
		)
	}
	if effect.Direction == wager.DirectionDebit {
		err = wallet.Debit(effect.Money)
	} else {
		err = wallet.Credit(effect.Money)
	}
	if err != nil {
		code := "INSUFFICIENT_BALANCE"
		if transaction.Kind() == wager.KindRefund || transaction.Kind() == wager.KindRollback {
			code = "REVERSAL_INSUFFICIENT_BALANCE"
		}
		return s.rejectTransaction(ctx, tx, transaction, wallet.Balance(), code, result)
	}
	after := wallet.Balance()
	if err := s.wallets.UpdateWallet(ctx, tx, wallet.ID(), after.MinorUnits(), wallet.Version()); err != nil {
		return err
	}
	direction := ledger.DirectionDebit
	if effect.Direction == wager.DirectionCredit {
		direction = ledger.DirectionCredit
	}
	entry, err := ledger.New(ledger.EntryInput{
		ID:            uuid.NewString(),
		WalletID:      transaction.WalletID(),
		TransactionID: transaction.ID(),
		Direction:     direction,
		Amount:        effect.Money,
		BalanceBefore: current,
		BalanceAfter:  after,
	})
	if err != nil {
		return err
	}
	if err := s.wagers.InsertLedger(ctx, tx, ports.LedgerRecord{
		ID:            entry.ID(),
		WalletID:      entry.WalletID(),
		TransactionID: entry.TransactionID(),
		Direction:     string(entry.Direction()),
		Amount:        entry.Amount().MinorUnits(),
		Currency:      entry.Amount().Currency(),
		BalanceBefore: entry.BalanceBefore().MinorUnits(),
		BalanceAfter:  entry.BalanceAfter().MinorUnits(),
	}); err != nil {
		return err
	}
	if err := transaction.MarkProcessed(after); err != nil {
		return err
	}
	if err := s.persistTransaction(ctx, tx, transaction); err != nil {
		return err
	}
	*result = resultFromTransaction(transaction)
	if err := s.outbox(
		ctx,
		tx,
		transaction,
		contracts.EventWagerTransactionProcessed,
		contracts.WagerTransactionProcessedData{
			TransactionID: transaction.ID(),
			WalletID:      transaction.WalletID(),
			Kind:          string(transaction.Kind()),
			Status:        string(transaction.Status()),
			Balance:       moneyPayload(after),
		},
	); err != nil {
		return err
	}
	return s.outbox(
		ctx,
		tx,
		transaction,
		contracts.EventWalletBalanceChanged,
		contracts.WalletBalanceChangedData{
			WalletID:      transaction.WalletID(),
			TransactionID: transaction.ID(),
			Direction:     string(direction),
			Money:         moneyPayload(effect.Money),
			BalanceBefore: moneyPayload(current),
			BalanceAfter:  moneyPayload(after),
			WalletVersion: wallet.Version(),
		},
	)
}

func (s *Service) resolveReference(
	ctx context.Context,
	tx pgx.Tx,
	record ports.WagerRecord,
	transaction wager.Transaction,
) (ports.WagerRecord, error) {
	referenceExternalID := transaction.ReferenceExternalTransactionID()
	if referenceExternalID == nil {
		return ports.WagerRecord{}, wager.ErrReferenceNotValid
	}
	reference, err := s.wagers.FindReferenceForUpdate(ctx, tx, providerID(record), *referenceExternalID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.WagerRecord{}, application.ErrPending
		}
		return ports.WagerRecord{}, err
	}
	if reference.Status != wager.StatusProcessed {
		return ports.WagerRecord{}, errors.New("reference not processed")
	}
	referenceEntity, err := rehydrateTransaction(reference)
	if err != nil {
		return ports.WagerRecord{}, err
	}
	if err := transaction.ValidateReference(referenceEntity); err != nil {
		return ports.WagerRecord{}, err
	}
	exists, err := s.wagers.HasSuccessfulReversal(
		ctx,
		tx,
		providerID(record),
		transaction.Kind(),
		*referenceExternalID,
	)
	if err != nil {
		return ports.WagerRecord{}, err
	}
	if exists {
		return ports.WagerRecord{}, errors.New("duplicate reversal")
	}
	return reference, nil
}

func (s *Service) rejectTransaction(
	ctx context.Context,
	tx pgx.Tx,
	transaction wager.Transaction,
	balance money.Money,
	code string,
	result *Result,
) error {
	if err := transaction.Reject(code, balance); err != nil {
		return err
	}
	if err := s.persistTransaction(ctx, tx, transaction); err != nil {
		return err
	}
	*result = resultFromTransaction(transaction)
	return s.outbox(
		ctx,
		tx,
		transaction,
		contracts.EventWagerTransactionRejected,
		contracts.WagerTransactionRejectedData{
			TransactionID: transaction.ID(),
			WalletID:      transaction.WalletID(),
			Status:        string(transaction.Status()),
			FailureCode:   code,
			Balance:       moneyPayload(balance),
		},
	)
}

func (s *Service) persistTransaction(
	ctx context.Context,
	tx pgx.Tx,
	transaction wager.Transaction,
) error {
	return s.wagers.UpdateWager(ctx, tx, wagerRecordFromTransaction(transaction))
}

func (s *Service) outbox(
	ctx context.Context,
	tx pgx.Tx,
	transaction wager.Transaction,
	eventType contracts.EventType,
	data any,
) error {
	eventID := contracts.NewEventID()
	payload, err := contracts.MarshalEvent(eventID, eventType, transaction.WalletID(), transaction.ID(), 1, data)
	if err != nil {
		return err
	}
	return s.wagers.InsertOutbox(ctx, tx, eventID, "wallet", transaction.WalletID(), string(eventType), payload)
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
	return contracts.MoneyInput{Amount: value.String(), Currency: value.Currency()}
}

func resultFromTransaction(transaction wager.Transaction) Result {
	result := Result{
		TransactionID: transaction.ID(),
		Status:        transaction.Status(),
	}
	if balance := transaction.ResultBalance(); balance != nil {
		value := *balance
		result.Balance = &value
	}
	if code := transaction.FailureCode(); code != nil {
		result.FailureCode = *code
	}
	return result
}

func wagerRecordFromTransaction(transaction wager.Transaction) ports.WagerRecord {
	var resultBalance *int64
	if balance := transaction.ResultBalance(); balance != nil {
		value := balance.MinorUnits()
		resultBalance = &value
	}
	return ports.WagerRecord{
		ID:                             transaction.ID(),
		ProviderID:                     transaction.ProviderID(),
		ExternalTransactionID:          transaction.ExternalTransactionID(),
		IdempotencyKey:                 transaction.IdempotencyKey(),
		WalletID:                       transaction.WalletID(),
		PlayerID:                       transaction.PlayerID(),
		RoundID:                        transaction.RoundID(),
		GameID:                         transaction.GameID(),
		Kind:                           transaction.Kind(),
		Amount:                         transaction.Money().MinorUnits(),
		Currency:                       transaction.Money().Currency(),
		ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
		ReferenceTransactionID:         transaction.ReferenceTransactionID(),
		Status:                         transaction.Status(),
		PayloadHash:                    transaction.PayloadHash(),
		ResultBalance:                  resultBalance,
		FailureCode:                    transaction.FailureCode(),
	}
}

func providerID(record ports.WagerRecord) string {
	if record.ProviderID == nil {
		return ""
	}
	return *record.ProviderID
}
