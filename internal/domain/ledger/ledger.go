package ledger

import (
	"errors"
	"time"

	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
)

var (
	ErrInvalidEntry       = errors.New("invalid ledger entry")
	ErrInvalidDirection   = errors.New("invalid ledger direction")
	ErrInvalidBalanceFlow = errors.New("invalid ledger balance flow")
)

type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

type EntryInput struct {
	ID            string
	WalletID      string
	TransactionID string
	Direction     Direction
	Amount        money.Money
	BalanceBefore money.Money
	BalanceAfter  money.Money
	CreatedAt     time.Time
}

type Entry struct {
	id            string
	walletID      string
	transactionID string
	direction     Direction
	amount        money.Money
	balanceBefore money.Money
	balanceAfter  money.Money
	createdAt     time.Time
}

func New(input EntryInput) (Entry, error) {
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	if input.ID == "" || input.WalletID == "" || input.TransactionID == "" || input.Amount.MinorUnits() <= 0 {
		return Entry{}, ErrInvalidEntry
	}
	if input.Direction != DirectionDebit && input.Direction != DirectionCredit {
		return Entry{}, ErrInvalidDirection
	}
	if input.Amount.Currency() != input.BalanceBefore.Currency() ||
		input.Amount.Currency() != input.BalanceAfter.Currency() {
		return Entry{}, money.ErrCurrencyMismatch
	}
	if input.BalanceAfter.MinorUnits() < 0 {
		return Entry{}, ErrInvalidBalanceFlow
	}
	var expected money.Money
	var err error
	if input.Direction == DirectionDebit {
		expected, err = input.BalanceBefore.Sub(input.Amount)
	} else {
		expected, err = input.BalanceBefore.Add(input.Amount)
	}
	if err != nil {
		return Entry{}, err
	}
	if expected.MinorUnits() != input.BalanceAfter.MinorUnits() {
		return Entry{}, ErrInvalidBalanceFlow
	}
	return Entry{
		id: input.ID, walletID: input.WalletID, transactionID: input.TransactionID,
		direction: input.Direction, amount: input.Amount,
		balanceBefore: input.BalanceBefore, balanceAfter: input.BalanceAfter,
		createdAt: input.CreatedAt,
	}, nil
}

func Rehydrate(input EntryInput) (Entry, error) { return New(input) }

func (e Entry) ID() string { return e.id }

func (e Entry) WalletID() string { return e.walletID }

func (e Entry) TransactionID() string { return e.transactionID }

func (e Entry) Direction() Direction { return e.direction }

func (e Entry) Amount() money.Money { return e.amount }

func (e Entry) BalanceBefore() money.Money { return e.balanceBefore }

func (e Entry) BalanceAfter() money.Money { return e.balanceAfter }

func (e Entry) CreatedAt() time.Time { return e.createdAt }
