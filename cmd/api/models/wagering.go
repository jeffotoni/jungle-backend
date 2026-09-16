package models

type WagerResponse struct {
	TransactionID    string `json:"transactionId"`
	Status           string `json:"status"`
	Balance          *Money `json:"balance,omitempty"`
	IdempotentReplay bool   `json:"idempotentReplay"`
	FailureCode      string `json:"failureCode,omitempty"`
}

type ReconciliationResponse struct {
	WalletID          string `json:"walletId"`
	StoredBalance     Money  `json:"storedBalance"`
	CalculatedBalance Money  `json:"calculatedBalance"`
	Difference        Money  `json:"difference"`
	Consistent        bool   `json:"consistent"`
	CheckedEntries    int64  `json:"checkedEntries"`
}
