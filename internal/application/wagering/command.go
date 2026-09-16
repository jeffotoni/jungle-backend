package wagering

import (
	"time"

	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/wager"
)

type ProcessCommand struct {
	ProviderID             string
	ExternalTransactionID  string
	WalletID               string
	PlayerID               string
	RoundID                string
	GameID                 string
	Kind                   wager.Kind
	Money                  money.Money
	ReferenceTransactionID *string
	OccurredAt             time.Time
	PayloadHash            string
}
