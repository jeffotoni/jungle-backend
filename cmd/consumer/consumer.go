package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/jackc/pgx/v5"

	"github.com/jeffotoni/jungle-backend-challenge/internal/application"
	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
	"github.com/jeffotoni/jungle-backend-challenge/internal/contracts"
)

type sqsClient interface {
	ReceiveMessageWithContext(context.Context, *sqs.ReceiveMessageInput, ...request.Option) (*sqs.ReceiveMessageOutput, error)
	DeleteMessageWithContext(context.Context, *sqs.DeleteMessageInput, ...request.Option) (*sqs.DeleteMessageOutput, error)
}

type messageEnvelope struct {
	MessageID  string                 `json:"messageId"`
	Type       string                 `json:"type"`
	OccurredAt time.Time              `json:"occurredAt"`
	Data       contracts.WagerRequest `json:"data"`
}

var (
	errPermanentMessage = errors.New("permanent SQS message error")
	errDuplicateMessage = errors.New("already processed SQS message")
)

func (c *Consumer) run(ctx context.Context) {
	defer close(c.done)
	for {
		output, err := c.client.ReceiveMessageWithContext(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(c.queueURL),
			MaxNumberOfMessages: aws.Int64(c.maxMessages),
			WaitTimeSeconds:     aws.Int64(c.waitSecond),
			VisibilityTimeout:   aws.Int64(c.visibilitySecond),
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logError(ctx, "receive", err, false, "")
			if !waitForRetry(ctx, time.Second) {
				return
			}
			continue
		}
		if len(output.Messages) > 0 {
			_ = c.logger.Debug().
				Component("sqs").
				Action("receive").
				Int("messages", len(output.Messages)).
				Msg("messages received").
				Send()
		}
		for _, message := range output.Messages {
			result, err := c.handleMessage(ctx, message)
			if err != nil && !errors.Is(err, errDuplicateMessage) {
				c.logError(ctx, "process", err, errors.Is(err, errPermanentMessage), result.MessageID)
				continue
			}
			if result.Duplicate {
				_ = c.logger.Info().
					Component("sqs").
					Action("duplicate").
					Str("messageId", result.MessageID).
					Bool("duplicate", true).
					Msg("duplicate message ignored").
					Send()
			} else {
				_ = c.logger.Info().
					Component("sqs").
					Action("process").
					Str("messageId", result.MessageID).
					Str("transactionId", result.TransactionID).
					Str("status", result.Status).
					Bool("duplicate", false).
					Msg("message processed").
					Send()
			}
			c.deleteMessage(ctx, message)
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Consumer) deleteMessage(ctx context.Context, message *sqs.Message) error {
	_, err := c.client.DeleteMessageWithContext(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: message.ReceiptHandle,
	})
	if err != nil {
		c.logError(ctx, "delete", err, false, "")
	} else {
		_ = c.logger.Debug().
			Component("sqs").
			Action("delete").
			Msg("message deleted").
			Send()
	}
	return err
}

type messageResult struct {
	MessageID     string
	TransactionID string
	Status        string
	Duplicate     bool
}

func (c *Consumer) handleMessage(ctx context.Context, message *sqs.Message) (messageResult, error) {
	if message == nil || message.Body == nil {
		return messageResult{}, fmt.Errorf("%w: empty message body", errPermanentMessage)
	}
	var envelope messageEnvelope
	if err := json.Unmarshal([]byte(*message.Body), &envelope); err != nil {
		return messageResult{}, fmt.Errorf("%w: invalid envelope: %v", errPermanentMessage, err)
	}
	if strings.TrimSpace(envelope.MessageID) == "" ||
		envelope.Type != "WagerTransactionRequested" ||
		envelope.OccurredAt.IsZero() {
		return messageResult{MessageID: envelope.MessageID}, fmt.Errorf("%w: incomplete envelope", errPermanentMessage)
	}
	hash, err := contracts.CanonicalHash(envelope.Data)
	if err != nil {
		return messageResult{MessageID: envelope.MessageID}, fmt.Errorf("%w: payload hash: %v", errPermanentMessage, err)
	}

	result := messageResult{MessageID: envelope.MessageID}
	err = c.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		existing, findErr := c.inbox.FindInbox(ctx, tx, c.consumerName, envelope.MessageID)
		if findErr == nil {
			if existing.PayloadHash != hash {
				return fmt.Errorf("%w: inbox payload mismatch", errPermanentMessage)
			}
			result.Duplicate = true
			return errDuplicateMessage
		}
		if !errors.Is(findErr, pgx.ErrNoRows) {
			return findErr
		}
		inserted, err := c.inbox.InsertInbox(ctx, tx, ports.InboxRecord{
			ConsumerName: c.consumerName,
			MessageID:    envelope.MessageID,
			PayloadHash:  hash,
			Status:       "PROCESSING",
		})
		if err != nil {
			return err
		}
		if !inserted {
			existing, err := c.inbox.FindInbox(ctx, tx, c.consumerName, envelope.MessageID)
			if err != nil {
				return err
			}
			if existing.PayloadHash != hash {
				return fmt.Errorf("%w: inbox payload mismatch", errPermanentMessage)
			}
			return errDuplicateMessage
		}
		processed, err := c.wagers.ProcessInTx(ctx, tx, envelope.Data)
		if err != nil {
			if errors.Is(err, application.ErrInvalid) ||
				errors.Is(err, application.ErrConflict) ||
				errors.Is(err, application.ErrNotFound) {
				return fmt.Errorf("%w: %v", errPermanentMessage, err)
			}
			return err
		}
		result.TransactionID = processed.TransactionID
		result.Status = string(processed.Status)
		return c.inbox.CompleteInbox(ctx, tx, c.consumerName, envelope.MessageID)
	})
	var permanent *application.PermanentFailure
	if errors.As(err, &permanent) {
		return c.persistPermanentFailure(ctx, envelope, hash, result, permanent.Code)
	}
	return result, err
}

func (c *Consumer) persistPermanentFailure(
	ctx context.Context,
	envelope messageEnvelope,
	hash string,
	result messageResult,
	code string,
) (messageResult, error) {
	err := c.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		existing, findErr := c.inbox.FindInbox(ctx, tx, c.consumerName, envelope.MessageID)
		if findErr == nil {
			if existing.PayloadHash != hash {
				return fmt.Errorf("%w: inbox payload mismatch", errPermanentMessage)
			}
			result.Duplicate = true
			return errDuplicateMessage
		}
		if !errors.Is(findErr, pgx.ErrNoRows) {
			return findErr
		}
		inserted, err := c.inbox.InsertInbox(ctx, tx, ports.InboxRecord{
			ConsumerName: c.consumerName,
			MessageID:    envelope.MessageID,
			PayloadHash:  hash,
			Status:       "PROCESSING",
		})
		if err != nil {
			return err
		}
		if !inserted {
			return errDuplicateMessage
		}
		failed, err := c.wagers.PersistPermanentFailureInTx(ctx, tx, envelope.Data, code)
		if err != nil {
			return err
		}
		result.TransactionID = failed.TransactionID
		result.Status = string(failed.Status)
		return c.inbox.CompleteInbox(ctx, tx, c.consumerName, envelope.MessageID)
	})
	return result, err
}

func (c *Consumer) logError(ctx context.Context, action string, err error, permanent bool, messageID string) {
	entry := c.logger.Error().
		Ctx(ctx).
		Component("sqs").
		Action(action).
		Err("error", err).
		Bool("permanent", permanent)
	if messageID != "" {
		entry.Str("messageId", messageID)
	}
	_ = entry.Msg("consumer processing failed").
		Send()
}
