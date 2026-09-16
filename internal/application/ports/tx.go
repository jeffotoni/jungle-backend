package ports

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jeffotoni/jungle-backend-challenge/internal/domain/wager"
)

var ErrUniqueViolation = errors.New("unique constraint violation")

type TxManager interface {
	WithinTx(ctx context.Context, fn func(pgx.Tx) error) error
}

type WalletRecord struct {
	ID        string
	PlayerID  string
	Balance   int64
	Currency  string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

type WagerRecord struct {
	ID                             string
	ProviderID                     *string
	ExternalTransactionID          *string
	IdempotencyKey                 *string
	WalletID                       string
	PlayerID                       string
	RoundID                        string
	GameID                         string
	Kind                           wager.Kind
	Amount                         int64
	Currency                       string
	ReferenceExternalTransactionID *string
	ReferenceTransactionID         *string
	Status                         wager.Status
	PayloadHash                    string
	ResultBalance                  *int64
	FailureCode                    *string
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

type InboxRecord struct {
	ConsumerName string
	MessageID    string
	PayloadHash  string
	Status       string
}

type OutboxRecord struct {
	ID            string
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	Status        string
	Attempts      int
	NextAttemptAt *time.Time
	PublishedAt   *time.Time
	ClaimToken    string
}

type LedgerRecord struct {
	ID            string
	WalletID      string
	TransactionID string
	Direction     string
	Amount        int64
	Currency      string
	BalanceBefore int64
	BalanceAfter  int64
	CreatedAt     time.Time
}

type WalletStore interface {
	CreateWallet(context.Context, pgx.Tx, WalletRecord) error
	GetWallet(context.Context, pgx.Tx, string) (WalletRecord, error)
	LockWallet(context.Context, pgx.Tx, string) (WalletRecord, error)
	UpdateWallet(context.Context, pgx.Tx, string, int64, int64) error
	ListLedger(context.Context, string, string, int) ([]LedgerRecord, error)
	LedgerBalance(context.Context, pgx.Tx, string) (int64, int64, error)
}

type WagerStore interface {
	FindByID(context.Context, pgx.Tx, string, string) (WagerRecord, error)
	FindByIdempotency(context.Context, pgx.Tx, string, string) (WagerRecord, error)
	FindByBusiness(context.Context, pgx.Tx, string, string) (WagerRecord, error)
	FindReference(context.Context, pgx.Tx, string, string) (WagerRecord, error)
	FindReferenceForUpdate(context.Context, pgx.Tx, string, string) (WagerRecord, error)
	HasSuccessfulReversal(context.Context, pgx.Tx, string, wager.Kind, string) (bool, error)
	InsertWager(context.Context, pgx.Tx, WagerRecord) (bool, error)
	UpdateWager(context.Context, pgx.Tx, WagerRecord) error
	InsertLedger(context.Context, pgx.Tx, LedgerRecord) error
	InsertOutbox(context.Context, pgx.Tx, string, string, string, []byte) error
}

type InboxStore interface {
	FindInbox(context.Context, pgx.Tx, string, string) (InboxRecord, error)
	InsertInbox(context.Context, pgx.Tx, InboxRecord) (bool, error)
	CompleteInbox(context.Context, pgx.Tx, string, string) error
}

type OutboxStore interface {
	ClaimOutbox(context.Context, pgx.Tx, int, string, time.Duration) ([]OutboxRecord, error)
	MarkOutboxPublished(context.Context, pgx.Tx, string, string) error
	MarkOutboxRetry(context.Context, pgx.Tx, string, string, time.Time) error
}
