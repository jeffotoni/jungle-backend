package contracts

import "time"

type MoneyInput struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type WagerRequest struct {
	ProviderID             string      `json:"providerId"`
	ExternalTransactionID  string      `json:"externalTransactionId"`
	WalletID               string      `json:"walletId"`
	PlayerID               string      `json:"playerId"`
	RoundID                string      `json:"roundId"`
	GameID                 string      `json:"gameId"`
	Kind                   string      `json:"kind"`
	Money                  MoneyInput  `json:"money"`
	ReferenceTransactionID *string     `json:"referenceTransactionId"`
	OccurredAt             time.Time   `json:"occurredAt"`
}
