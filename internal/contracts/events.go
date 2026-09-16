package contracts

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EventWagerTransactionProcessed        EventType = "WagerTransactionProcessed"
	EventWagerTransactionRejected         EventType = "WagerTransactionRejected"
	EventWalletBalanceChanged             EventType = "WalletBalanceChanged"
	EventWagerTransactionPendingReference EventType = "WagerTransactionPendingReference"
)

type EventEnvelope[T any] struct {
	EventID       string    `json:"eventId"`
	EventType     EventType `json:"eventType"`
	AggregateID   string    `json:"aggregateId"`
	CorrelationID string    `json:"correlationId"`
	CausationID   *string   `json:"causationId,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
	Version       int64     `json:"version"`
	Data          T         `json:"data"`
}

type WagerTransactionProcessedData struct {
	TransactionID string     `json:"transactionId"`
	WalletID      string     `json:"walletId"`
	Kind          string     `json:"kind"`
	Status        string     `json:"status"`
	Balance       MoneyInput `json:"balance"`
}

type WagerTransactionRejectedData struct {
	TransactionID string     `json:"transactionId"`
	WalletID      string     `json:"walletId"`
	Status        string     `json:"status"`
	FailureCode   string     `json:"failureCode"`
	Balance       MoneyInput `json:"balance"`
}

type WagerTransactionPendingReferenceData struct {
	TransactionID string `json:"transactionId"`
	WalletID      string `json:"walletId"`
	Status        string `json:"status"`
	FailureCode   string `json:"failureCode"`
}

type WalletBalanceChangedData struct {
	WalletID      string     `json:"walletId"`
	TransactionID string     `json:"transactionId"`
	Direction     string     `json:"direction"`
	Money         MoneyInput `json:"money"`
	BalanceBefore MoneyInput `json:"balanceBefore"`
	BalanceAfter  MoneyInput `json:"balanceAfter"`
	WalletVersion int64      `json:"walletVersion"`
}

func MarshalEvent[T any](eventType EventType, aggregateID, correlationID string, version int64, data T) ([]byte, error) {
	return json.Marshal(EventEnvelope[T]{
		EventID:       newEventID(),
		EventType:     eventType,
		AggregateID:   aggregateID,
		CorrelationID: correlationID,
		OccurredAt:    time.Now().UTC(),
		Version:       version,
		Data:          data,
	})
}

func newEventID() string {
	return uuid.NewString()
}
