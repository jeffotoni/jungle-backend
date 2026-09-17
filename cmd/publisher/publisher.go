package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jeffotoni/log"
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend/cmd/publisher/config"
	"github.com/jeffotoni/jungle-backend/internal/application/ports"
)

type sqsClient interface {
	GetQueueAttributesWithContext(context.Context, *sqs.GetQueueAttributesInput, ...request.Option) (*sqs.GetQueueAttributesOutput, error)
	SendMessageWithContext(context.Context, *sqs.SendMessageInput, ...request.Option) (*sqs.SendMessageOutput, error)
}

type Publisher struct {
	client        sqsClient
	tx            ports.TxManager
	outbox        ports.OutboxStore
	logger        *log.Logger
	queueURL      string
	publisherName string
	batchSize     int
	pollInterval  time.Duration
	lease         time.Duration
	retryBase     time.Duration
	retryMax      time.Duration
	groupID       string
	cancel        context.CancelFunc
	done          chan struct{}
}

func NewPublisher(
	lc fx.Lifecycle,
	client *sqs.SQS,
	tx ports.TxManager,
	outbox ports.OutboxStore,
	logger *log.Logger,
) *Publisher {
	p := &Publisher{
		client:        client,
		tx:            tx,
		outbox:        outbox,
		logger:        logger,
		queueURL:      config.SQS_EVENTS_QUEUE_URL,
		publisherName: config.PUBLISHER_NAME,
		batchSize:     config.OUTBOX_BATCH_SIZE,
		pollInterval:  config.POLL_INTERVAL,
		lease:         config.PUBLISHER_LEASE,
		retryBase:     config.RETRY_BASE,
		retryMax:      config.RETRY_MAX,
		groupID:       config.SQS_EVENT_GROUP_ID,
		done:          make(chan struct{}),
	}
	if p.batchSize <= 0 {
		p.batchSize = 100
	}
	if p.pollInterval <= 0 {
		p.pollInterval = time.Second
	}
	if p.lease <= 0 {
		p.lease = 30 * time.Second
	}
	if p.retryBase <= 0 {
		p.retryBase = time.Second
	}
	if p.retryMax < p.retryBase {
		p.retryMax = time.Minute
	}
	if strings.TrimSpace(p.publisherName) == "" {
		p.publisherName = "outbox-publisher"
	}
	if strings.TrimSpace(p.groupID) == "" {
		p.groupID = "outbox-events"
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if strings.TrimSpace(p.queueURL) == "" {
				return errors.New("SQS_EVENTS_QUEUE_URL is required")
			}
			if _, err := p.client.GetQueueAttributesWithContext(ctx, &sqs.GetQueueAttributesInput{
				QueueUrl:       aws.String(p.queueURL),
				AttributeNames: []*string{aws.String("QueueArn")},
			}); err != nil {
				return errors.New("SQS event queue unavailable")
			}
			workerContext, cancel := context.WithCancel(context.Background())
			p.cancel = cancel
			_ = p.logger.Info().
				Component("outbox").
				Action("startup").
				Str("queueUrl", p.queueURL).
				Str("publisherName", p.publisherName).
				Msg("publisher started").
				Send()
			go p.run(workerContext)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if p.cancel != nil {
				p.cancel()
			}
			select {
			case <-p.done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	return p
}

func (p *Publisher) run(ctx context.Context) {
	defer close(p.done)
	for {
		if ctx.Err() != nil {
			return
		}
		records, err := p.claim(ctx)
		if err != nil {
			p.logError(ctx, "claim", err, "")
			if !waitForRetry(ctx, p.retryBase) {
				return
			}
			continue
		}
		if len(records) == 0 {
			if !waitForRetry(ctx, p.pollInterval) {
				return
			}
			continue
		}
		for _, record := range records {
			if !p.publish(ctx, record) && ctx.Err() != nil {
				return
			}
		}
	}
}

func (p *Publisher) claim(ctx context.Context) ([]ports.OutboxRecord, error) {
	claimToken := p.publisherName + ":" + uuid.NewString()
	var records []ports.OutboxRecord
	err := p.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		var err error
		records, err = p.outbox.ClaimOutbox(ctx, tx, p.batchSize, claimToken, p.lease)
		return err
	})
	if err != nil {
		return nil, err
	}
	for index := range records {
		records[index].ClaimToken = claimToken
	}
	return records, nil
}

func (p *Publisher) publish(ctx context.Context, record ports.OutboxRecord) bool {
	output, err := p.client.SendMessageWithContext(ctx, p.messageInput(record))
	if err != nil {
		p.markRetry(ctx, record, err)
		return false
	}
	if err := p.markPublished(ctx, record); err != nil {
		p.logError(ctx, "mark_published", err, record.ID)
		return false
	}
	entry := p.logger.Info().
		Ctx(ctx).
		Component("outbox").
		Action("publish").
		Str("eventId", record.ID).
		Str("eventType", record.EventType)
	if output != nil && output.MessageId != nil {
		entry.Str("messageId", aws.StringValue(output.MessageId))
	}
	_ = entry.Msg("outbox event published").Send()
	return true
}

func (p *Publisher) messageInput(record ports.OutboxRecord) *sqs.SendMessageInput {
	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(p.queueURL),
		MessageBody: aws.String(string(record.Payload)),
	}
	if strings.HasSuffix(strings.TrimRight(p.queueURL, "/"), ".fifo") {
		input.MessageGroupId = aws.String(p.groupID)
		input.MessageDeduplicationId = aws.String(record.ID)
	}
	return input
}

func (p *Publisher) markPublished(ctx context.Context, record ports.OutboxRecord) error {
	return p.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		return p.outbox.MarkOutboxPublished(ctx, tx, record.ID, record.ClaimToken)
	})
}

func (p *Publisher) markRetry(ctx context.Context, record ports.OutboxRecord, publishErr error) {
	nextAttempt := time.Now().UTC().Add(retryDelay(p.retryBase, p.retryMax, record.Attempts))
	err := p.tx.WithinTx(ctx, func(tx pgx.Tx) error {
		return p.outbox.MarkOutboxRetry(ctx, tx, record.ID, record.ClaimToken, nextAttempt)
	})
	if err != nil {
		p.logError(ctx, "retry_state", err, record.ID)
		return
	}
	_ = p.logger.Warn().
		Ctx(ctx).
		Component("outbox").
		Action("retry").
		Str("eventId", record.ID).
		Str("eventType", record.EventType).
		Int("attempts", record.Attempts+1).
		Str("nextAttemptAt", nextAttempt.Format(time.RFC3339Nano)).
		Err("error", publishErr).
		Msg("outbox event publish failed").
		Send()
}

func (p *Publisher) logError(ctx context.Context, action string, err error, eventID string) {
	entry := p.logger.Error().
		Ctx(ctx).
		Component("outbox").
		Action(action).
		Err("error", err)
	if eventID != "" {
		entry.Str("eventId", eventID)
	}
	_ = entry.Msg("publisher operation failed").Send()
}

func retryDelay(base, maximum time.Duration, attempts int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if maximum < base {
		maximum = base
	}
	if attempts < 0 {
		attempts = 0
	}
	if attempts > 30 {
		attempts = 30
	}
	delay := base
	for index := 0; index < attempts && delay < maximum; index++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
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
