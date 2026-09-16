package wager

import (
	"time"

	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/money"
)

type Kind string

const (
	KindBet      Kind = "BET"
	KindWin      Kind = "WIN"
	KindLoss     Kind = "LOSS"
	KindRefund   Kind = "REFUND"
	KindRollback Kind = "ROLLBACK"
)

type Status string

const (
	StatusPendingReference Status = "PENDING_REFERENCE"
	StatusProcessed        Status = "PROCESSED"
	StatusRejected         Status = "REJECTED"
)

type Transaction struct {
	ID                     string
	ProviderID             string
	ExternalTransactionID  string
	WalletID               string
	PlayerID               string
	RoundID                string
	GameID                 string
	Kind                   Kind
	Money                  money.Money
	ReferenceTransactionID *string
	OccurredAt             time.Time
	Status                 Status
	PayloadHash            string
}
