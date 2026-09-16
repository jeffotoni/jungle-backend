package wallet

import "github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"

type Wallet struct {
	ID       string
	PlayerID string
	Balance  money.Money
}
