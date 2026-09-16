package wallet

import (
	"errors"

	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
)

var ErrInvalidWallet = errors.New("invalid wallet")

type Wallet struct {
	ID       string
	PlayerID string
	Balance  money.Money
	Version  int64
}

func New(id, playerID string, balance money.Money) (Wallet, error) {
	if id == "" || playerID == "" || balance.Amount < 0 {
		return Wallet{}, ErrInvalidWallet
	}
	return Wallet{ID: id, PlayerID: playerID, Balance: balance, Version: 1}, nil
}

func Rehydrate(id, playerID string, balance money.Money, version int64) (Wallet, error) {
	if id == "" || playerID == "" || balance.Amount < 0 || version < 1 {
		return Wallet{}, ErrInvalidWallet
	}
	return Wallet{ID: id, PlayerID: playerID, Balance: balance, Version: version}, nil
}
