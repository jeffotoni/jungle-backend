package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend/internal/application/ports"
)

var ErrOutboxClaimLost = errors.New("outbox claim lost")

func (s *Store) ClaimOutbox(
	ctx context.Context,
	tx pgx.Tx,
	limit int,
	claimToken string,
	lease time.Duration,
) ([]ports.OutboxRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if strings.TrimSpace(claimToken) == "" {
		return nil, errors.New("outbox claim token is required")
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}

	rows, err := tx.Query(ctx, `
		SELECT id, aggregate_type, aggregate_id, event_type, payload,
		       status, attempts, next_attempt_at, published_at
		FROM outbox_events
		WHERE (
			(status = 'PENDING' AND (next_attempt_at IS NULL OR next_attempt_at <= now()))
			OR
			(status = 'PROCESSING' AND (lease_until IS NULL OR lease_until <= now()))
		)
		ORDER BY created_at ASC, id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}

	records := make([]ports.OutboxRecord, 0, limit)
	for rows.Next() {
		var record ports.OutboxRecord
		var id uuid.UUID
		if err := rows.Scan(
			&id,
			&record.AggregateType,
			&record.AggregateID,
			&record.EventType,
			&record.Payload,
			&record.Status,
			&record.Attempts,
			&record.NextAttemptAt,
			&record.PublishedAt,
		); err != nil {
			rows.Close()
			return nil, err
		}
		record.ID = id.String()
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	leaseUntil := time.Now().UTC().Add(lease)
	for _, record := range records {
		result, err := tx.Exec(ctx, `
			UPDATE outbox_events
			SET status = 'PROCESSING', claim_token = $2, lease_until = $3
			WHERE id = $1`, mustUUID(record.ID), claimToken, leaseUntil)
		if err != nil {
			return nil, err
		}
		if result.RowsAffected() != 1 {
			return nil, ErrOutboxClaimLost
		}
	}

	return records, nil
}

func (s *Store) MarkOutboxPublished(ctx context.Context, tx pgx.Tx, id, claimToken string) error {
	result, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'PUBLISHED', published_at = now(), next_attempt_at = NULL,
		    claim_token = NULL, lease_until = NULL
		WHERE id = $1 AND status = 'PROCESSING' AND claim_token = $2`,
		mustUUID(id), claimToken)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrOutboxClaimLost
	}
	return nil
}

func (s *Store) MarkOutboxRetry(ctx context.Context, tx pgx.Tx, id, claimToken string, nextAttemptAt time.Time) error {
	result, err := tx.Exec(ctx, `
		UPDATE outbox_events
		SET status = 'PENDING', attempts = attempts + 1, next_attempt_at = $3,
		    claim_token = NULL, lease_until = NULL
		WHERE id = $1 AND status = 'PROCESSING' AND claim_token = $2`,
		mustUUID(id), claimToken, nextAttemptAt.UTC())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrOutboxClaimLost
	}
	return nil
}
