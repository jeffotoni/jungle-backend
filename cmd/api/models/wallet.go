package models

type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type CreateWalletRequest struct {
	PlayerID       string `json:"playerId"`
	InitialBalance Money  `json:"initialBalance"`
}

type WalletResponse struct {
	ID       string `json:"id"`
	PlayerID string `json:"playerId"`
	Balance  Money  `json:"balance"`
	Version  int64  `json:"version"`
}

type LedgerEntryResponse struct {
	ID            string `json:"id"`
	TransactionID string `json:"transactionId"`
	Direction     string `json:"direction"`
	Money         Money  `json:"money"`
	BalanceBefore Money  `json:"balanceBefore"`
	BalanceAfter  Money  `json:"balanceAfter"`
	CreatedAt     string `json:"createdAt"`
}

type LedgerResponse struct {
	Entries    []LedgerEntryResponse `json:"entries"`
	NextCursor string                `json:"nextCursor,omitempty"`
}
