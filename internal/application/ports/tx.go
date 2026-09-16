package ports

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type TxManager interface {
	WithinTx(ctx context.Context, fn func(pgx.Tx) error) error
}
