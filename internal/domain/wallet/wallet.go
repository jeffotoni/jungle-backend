package wallet

import (
	"errors"

	"github.com/jeffotoni/jungle-backend/internal/domain/money"
)

var ErrInvalidWallet = errors.New("invalid wallet")
var ErrInsufficientBalance = errors.New("insufficient wallet balance")

type Wallet struct {
	id       string
	playerID string
	balance  money.Money
	version  int64
}

func New(id, playerID string, balance money.Money) (Wallet, error) {
	if id == "" || playerID == "" || balance.MinorUnits() < 0 {
		return Wallet{}, ErrInvalidWallet
	}
	return Wallet{id: id, playerID: playerID, balance: balance, version: 1}, nil
}

func Rehydrate(id, playerID string, balance money.Money, version int64) (Wallet, error) {
	if id == "" || playerID == "" || balance.MinorUnits() < 0 || version < 1 {
		return Wallet{}, ErrInvalidWallet
	}
	return Wallet{id: id, playerID: playerID, balance: balance, version: version}, nil
}

func (w Wallet) ID() string { return w.id }

func (w Wallet) PlayerID() string { return w.playerID }

func (w Wallet) Balance() money.Money { return w.balance }

func (w Wallet) Version() int64 { return w.version }

func (w *Wallet) Debit(amount money.Money) error {
	if amount.MinorUnits() <= 0 {
		return money.ErrInvalidAmount
	}
	if amount.Currency() != w.balance.Currency() {
		return money.ErrCurrencyMismatch
	}
	if w.balance.MinorUnits() < amount.MinorUnits() {
		return ErrInsufficientBalance
	}
	next, err := w.balance.Sub(amount)
	if err != nil {
		return err
	}
	w.balance = next
	w.version++
	return nil
}

func (w *Wallet) Credit(amount money.Money) error {
	if amount.MinorUnits() <= 0 {
		return money.ErrInvalidAmount
	}
	if amount.Currency() != w.balance.Currency() {
		return money.ErrCurrencyMismatch
	}
	next, err := w.balance.Add(amount)
	if err != nil {
		return err
	}
	w.balance = next
	w.version++
	return nil
}
