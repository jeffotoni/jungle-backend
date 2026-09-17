package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeffotoni/jungle-backend/internal/application/ports"
)

type txManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) ports.TxManager {
	return &txManager{pool: pool}
}

func (m *txManager) WithinTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
