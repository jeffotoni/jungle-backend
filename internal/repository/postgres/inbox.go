package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend/internal/application/ports"
)

func (s *Store) FindInbox(ctx context.Context, tx pgx.Tx, consumerName, messageID string) (ports.InboxRecord, error) {
	var record ports.InboxRecord
	err := tx.QueryRow(ctx, `
		SELECT consumer_name, message_id, payload_hash, status
		FROM inbox_messages
		WHERE consumer_name = $1 AND message_id = $2`, consumerName, messageID).
		Scan(&record.ConsumerName, &record.MessageID, &record.PayloadHash, &record.Status)
	return record, err
}

func (s *Store) InsertInbox(ctx context.Context, tx pgx.Tx, record ports.InboxRecord) (bool, error) {
	result, err := tx.Exec(ctx, `
		INSERT INTO inbox_messages (consumer_name, message_id, payload_hash, status)
		VALUES ($1, $2, $3, 'PROCESSING')
		ON CONFLICT (consumer_name, message_id) DO NOTHING`,
		record.ConsumerName, record.MessageID, record.PayloadHash)
	return result.RowsAffected() == 1, err
}

func (s *Store) CompleteInbox(ctx context.Context, tx pgx.Tx, consumerName, messageID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE inbox_messages
		SET status = 'COMPLETED', processed_at = now()
		WHERE consumer_name = $1 AND message_id = $2`, consumerName, messageID)
	return err
}
