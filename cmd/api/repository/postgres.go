package repository

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jeffotoni/jungle-backend/internal/application/ports"
)

type Store struct {
	pool *pgxpool.Pool
}

var ErrInvalidCursor = errors.New("invalid cursor")

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func parseUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID: %w", err)
	}
	return id, nil
}

func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

var _ ports.WalletStore = (*Store)(nil)
var _ ports.WagerStore = (*Store)(nil)
