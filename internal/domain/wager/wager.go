package wager

import (
	"errors"
	"time"

	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
)

var (
	ErrInvalidTransaction = errors.New("invalid wagering transaction")
	ErrInvalidTransition  = errors.New("invalid wagering transition")
	ErrReferenceMismatch  = errors.New("reference mismatch")
	ErrReferenceNotValid  = errors.New("reference is not valid")
)

type Kind string

const (
	KindOpening  Kind = "OPENING"
	KindBet      Kind = "BET"
	KindWin      Kind = "WIN"
	KindLoss     Kind = "LOSS"
	KindRefund   Kind = "REFUND"
	KindRollback Kind = "ROLLBACK"
)

type Status string

const (
	StatusPending          Status = "PENDING"
	StatusPendingReference Status = "PENDING_REFERENCE"
	StatusProcessed        Status = "PROCESSED"
	StatusRejected         Status = "REJECTED"
	StatusFailed           Status = "FAILED"
)

type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

type Input struct {
	ID                             string
	ProviderID                     *string
	ExternalTransactionID          *string
	IdempotencyKey                 *string
	WalletID                       string
	PlayerID                       string
	RoundID                        string
	GameID                         string
	Kind                           Kind
	Money                          money.Money
	ReferenceExternalTransactionID *string
	ReferenceTransactionID         *string
	Status                         Status
	PayloadHash                    string
	ResultBalance                  *money.Money
	FailureCode                    *string
	OccurredAt                     time.Time
}

type Transaction struct {
	id                             string
	providerID                     *string
	externalTransactionID          *string
	idempotencyKey                 *string
	walletID                       string
	playerID                       string
	roundID                        string
	gameID                         string
	kind                           Kind
	money                          money.Money
	referenceExternalTransactionID *string
	referenceTransactionID         *string
	status                         Status
	payloadHash                    string
	resultBalance                  *money.Money
	failureCode                    *string
	occurredAt                     time.Time
}

func New(input Input) (Transaction, error) {
	if input.Status == "" {
		input.Status = StatusPending
	}
	return build(input, false)
}

func NewOpening(id, walletID, playerID string, value money.Money) (Transaction, error) {
	return build(Input{
		ID:            id,
		WalletID:      walletID,
		PlayerID:      playerID,
		Kind:          KindOpening,
		Money:         value,
		Status:        StatusProcessed,
		PayloadHash:   "internal",
		OccurredAt:    time.Now().UTC(),
		ResultBalance: &value,
	}, true)
}

func Rehydrate(input Input) (Transaction, error) {
	return build(input, input.Kind == KindOpening)
}

func build(input Input, opening bool) (Transaction, error) {
	if input.ID == "" || input.WalletID == "" || input.PlayerID == "" || input.Money.Currency() == "" {
		return Transaction{}, ErrInvalidTransaction
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Now().UTC()
	}
	if opening || input.Kind == KindOpening {
		if input.Kind != KindOpening || input.ProviderID != nil || input.ExternalTransactionID != nil ||
			input.IdempotencyKey != nil || input.ReferenceExternalTransactionID != nil ||
			input.ReferenceTransactionID != nil || input.FailureCode != nil || input.Money.MinorUnits() <= 0 ||
			input.Status != StatusProcessed || input.ResultBalance == nil ||
			input.ResultBalance.MinorUnits() != input.Money.MinorUnits() ||
			input.ResultBalance.Currency() != input.Money.Currency() {
			return Transaction{}, ErrInvalidTransaction
		}
		return Transaction{
			id: input.ID, walletID: input.WalletID, playerID: input.PlayerID,
			kind: input.Kind, money: input.Money, status: input.Status,
			payloadHash: input.PayloadHash, resultBalance: cloneMoney(input.ResultBalance),
			occurredAt: input.OccurredAt,
		}, nil
	}
	if input.ProviderID == nil || input.ExternalTransactionID == nil || input.IdempotencyKey == nil ||
		*input.ProviderID == "" || *input.ExternalTransactionID == "" || *input.IdempotencyKey == "" ||
		input.PayloadHash == "" || !validExternalKind(input.Kind) || !validStatus(input.Status) {
		return Transaction{}, ErrInvalidTransaction
	}
	if input.Money.MinorUnits() < 0 ||
		(input.Kind == KindLoss && input.Money.MinorUnits() != 0) ||
		(input.Kind != KindLoss && input.Money.MinorUnits() == 0) {
		return Transaction{}, ErrInvalidTransaction
	}
	if (input.Kind == KindRefund || input.Kind == KindRollback) && input.ReferenceExternalTransactionID == nil {
		return Transaction{}, ErrInvalidTransaction
	}
	if input.ReferenceExternalTransactionID != nil &&
		(input.Kind != KindWin && input.Kind != KindRefund && input.Kind != KindRollback) {
		return Transaction{}, ErrInvalidTransaction
	}
	return Transaction{
		id: input.ID, providerID: cloneString(input.ProviderID), externalTransactionID: cloneString(input.ExternalTransactionID),
		idempotencyKey: cloneString(input.IdempotencyKey), walletID: input.WalletID, playerID: input.PlayerID,
		roundID: input.RoundID, gameID: input.GameID, kind: input.Kind, money: input.Money,
		referenceExternalTransactionID: cloneString(input.ReferenceExternalTransactionID),
		referenceTransactionID:         cloneString(input.ReferenceTransactionID), status: input.Status,
		payloadHash: input.PayloadHash, resultBalance: cloneMoney(input.ResultBalance),
		failureCode: cloneString(input.FailureCode), occurredAt: input.OccurredAt,
	}, nil
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneMoney(value *money.Money) *money.Money {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func validExternalKind(kind Kind) bool {
	return kind == KindBet || kind == KindWin || kind == KindLoss || kind == KindRefund || kind == KindRollback
}

func validStatus(status Status) bool {
	return status == StatusPending || status == StatusPendingReference || status == StatusProcessed ||
		status == StatusRejected || status == StatusFailed
}

func (t Transaction) ID() string { return t.id }

func (t Transaction) ProviderID() *string { return cloneString(t.providerID) }

func (t Transaction) ExternalTransactionID() *string { return cloneString(t.externalTransactionID) }

func (t Transaction) IdempotencyKey() *string { return cloneString(t.idempotencyKey) }

func (t Transaction) WalletID() string { return t.walletID }

func (t Transaction) PlayerID() string { return t.playerID }

func (t Transaction) RoundID() string { return t.roundID }

func (t Transaction) GameID() string { return t.gameID }

func (t Transaction) Kind() Kind { return t.kind }

func (t Transaction) Money() money.Money { return t.money }

func (t Transaction) ReferenceExternalTransactionID() *string {
	return cloneString(t.referenceExternalTransactionID)
}

func (t Transaction) ReferenceTransactionID() *string { return cloneString(t.referenceTransactionID) }

func (t Transaction) Status() Status { return t.status }

func (t Transaction) PayloadHash() string { return t.payloadHash }

func (t Transaction) ResultBalance() *money.Money { return cloneMoney(t.resultBalance) }

func (t Transaction) FailureCode() *string { return cloneString(t.failureCode) }

func (t Transaction) OccurredAt() time.Time { return t.occurredAt }

func (t *Transaction) ResolveReference(id string) error {
	if t.status != StatusPending && t.status != StatusPendingReference {
		return ErrInvalidTransition
	}
	if id == "" {
		return ErrInvalidTransaction
	}
	t.referenceTransactionID = &id
	return nil
}

func (t *Transaction) MarkProcessed(balance money.Money) error {
	if t.status != StatusPending && t.status != StatusPendingReference {
		return ErrInvalidTransition
	}
	if balance.Currency() != t.money.Currency() || balance.MinorUnits() < 0 {
		return ErrInvalidTransaction
	}
	if (t.kind == KindRefund || t.kind == KindRollback) && t.referenceTransactionID == nil {
		return ErrReferenceNotValid
	}
	t.status = StatusProcessed
	t.resultBalance = &balance
	t.failureCode = nil
	return nil
}

func (t *Transaction) MarkPendingReference(balance money.Money) error {
	if (t.kind != KindRefund && t.kind != KindRollback) ||
		(t.status != StatusPending && t.status != StatusPendingReference) {
		return ErrInvalidTransition
	}
	if balance.Currency() != t.money.Currency() || balance.MinorUnits() < 0 {
		return ErrInvalidTransaction
	}
	t.status = StatusPendingReference
	t.resultBalance = &balance
	code := "REFERENCE_PENDING"
	t.failureCode = &code
	return nil
}

func (t *Transaction) Reject(code string, balance money.Money) error {
	if t.status != StatusPending && t.status != StatusPendingReference {
		return ErrInvalidTransition
	}
	if code == "" || balance.Currency() != t.money.Currency() || balance.MinorUnits() < 0 {
		return ErrInvalidTransaction
	}
	t.status = StatusRejected
	t.resultBalance = &balance
	t.failureCode = &code
	return nil
}

func (t *Transaction) Fail(code string, balance money.Money) error {
	if t.status != StatusPending && t.status != StatusPendingReference {
		return ErrInvalidTransition
	}
	if code == "" || balance.Currency() != t.money.Currency() || balance.MinorUnits() < 0 {
		return ErrInvalidTransaction
	}
	t.status = StatusFailed
	t.resultBalance = &balance
	t.failureCode = &code
	return nil
}

func (t Transaction) ValidateReference(reference Transaction) error {
	if reference.status != StatusProcessed || t.providerID == nil || reference.providerID == nil ||
		*reference.providerID != *t.providerID || reference.playerID != t.playerID ||
		reference.walletID != t.walletID || reference.money.Currency() != t.money.Currency() ||
		reference.roundID != t.roundID || reference.money.MinorUnits() != t.money.MinorUnits() {
		return ErrReferenceMismatch
	}
	switch t.kind {
	case KindWin, KindRefund:
		if reference.kind != KindBet {
			return ErrReferenceNotValid
		}
	case KindRollback:
		if reference.kind != KindBet && reference.kind != KindWin && reference.kind != KindRefund {
			return ErrReferenceNotValid
		}
	default:
		return ErrReferenceNotValid
	}
	return nil
}

type Effect struct {
	Direction Direction
	Money     money.Money
	Applied   bool
}

func (t Transaction) FinancialEffect(referenceKind *Kind) (Effect, error) {
	if t.status != StatusPending && t.status != StatusPendingReference {
		return Effect{}, ErrInvalidTransition
	}
	switch t.kind {
	case KindBet:
		return Effect{Direction: DirectionDebit, Money: t.money, Applied: true}, nil
	case KindWin, KindRefund:
		if t.kind == KindRefund && referenceKind == nil {
			return Effect{}, ErrReferenceNotValid
		}
		return Effect{Direction: DirectionCredit, Money: t.money, Applied: true}, nil
	case KindRollback:
		if referenceKind == nil {
			return Effect{}, ErrReferenceNotValid
		}
		if *referenceKind == KindBet {
			return Effect{Direction: DirectionCredit, Money: t.money, Applied: true}, nil
		}
		if *referenceKind == KindWin || *referenceKind == KindRefund {
			return Effect{Direction: DirectionDebit, Money: t.money, Applied: true}, nil
		}
		return Effect{}, ErrReferenceNotValid
	case KindLoss:
		return Effect{Money: t.money, Applied: false}, nil
	default:
		return Effect{}, ErrInvalidTransaction
	}
}
