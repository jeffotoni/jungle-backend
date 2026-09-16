package main

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/jeffotoni/log"
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend-challenge/cmd/consumer/config"
	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
	appwager "github.com/jeffotoni/jungle-backend-challenge/internal/application/wagering"
	"github.com/jeffotoni/jungle-backend-challenge/internal/repository/postgres"
)

func newSQSClient() (*sqs.SQS, error) {
	awsConfig := aws.NewConfig().WithRegion(config.AWS_REGION)
	if config.SQS_ENDPOINT_URL != "" {
		awsConfig = awsConfig.WithEndpoint(config.SQS_ENDPOINT_URL).
			WithCredentials(credentials.NewStaticCredentials("test", "test", ""))
	}
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, err
	}
	return sqs.New(sess), nil
}

func main() {
	fx.New(
		fx.NopLogger,
		fx.Provide(
			func() string { return config.DATABASE_URL },
			postgres.NewPool,
			postgres.NewTxManager,
			postgres.NewStore,
			func(store *postgres.Store) ports.WalletStore { return store },
			func(store *postgres.Store) ports.WagerStore { return store },
			func(store *postgres.Store) ports.InboxStore { return store },
			appwager.NewService,
			newSQSClient,
			func() *log.Logger {
				return log.New(log.Config{
					Format:      log.FormatJSON,
					Level:       log.Level(config.LOG_LEVEL),
					ServiceName: "consumer",
					TraceIDKey:  config.TRACE_ID,
				})
			},
		),
		fx.Provide(NewConsumer),
		fx.Invoke(func(*Consumer) {}),
	).Run()
}

type Consumer struct {
	client           sqsClient
	tx               ports.TxManager
	inbox            ports.InboxStore
	wagers           *appwager.Service
	logger           *log.Logger
	queueURL         string
	consumerName     string
	visibilitySecond int64
	waitSecond       int64
	maxMessages      int64
	cancel           context.CancelFunc
	done             chan struct{}
}

func NewConsumer(
	lc fx.Lifecycle,
	client *sqs.SQS,
	tx ports.TxManager,
	inbox ports.InboxStore,
	wagers *appwager.Service,
	logger *log.Logger,
) *Consumer {
	c := &Consumer{
		client:           client,
		tx:               tx,
		inbox:            inbox,
		wagers:           wagers,
		logger:           logger,
		queueURL:         config.SQS_WAGER_QUEUE_URL,
		consumerName:     config.CONSUMER_NAME,
		visibilitySecond: durationSeconds(config.VISIBILITY_TIMEOUT),
		waitSecond:       durationSeconds(config.SQS_WAIT_TIME),
		maxMessages:      int64(config.SQS_MAX_MESSAGES),
		done:             make(chan struct{}),
	}
	if c.visibilitySecond <= 0 {
		c.visibilitySecond = 30
	}
	if c.waitSecond <= 0 {
		c.waitSecond = 20
	}
	if c.maxMessages <= 0 || c.maxMessages > 10 {
		c.maxMessages = 10
	}
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if c.queueURL == "" {
				return errors.New("SQS_WAGER_QUEUE_URL is required")
			}
			ctx, cancel := context.WithCancel(context.Background())
			c.cancel = cancel
			_ = c.logger.Info().
				Component("sqs").
				Action("startup").
				Str("queueUrl", c.queueURL).
				Str("consumerName", c.consumerName).
				Msg("consumer started").
				Send()
			go c.run(ctx)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if c.cancel != nil {
				c.cancel()
			}
			select {
			case <-c.done:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	return c
}

func durationSeconds(value time.Duration) int64 {
	seconds := int64(value / time.Second)
	if value > 0 && seconds == 0 {
		return 1
	}
	return seconds
}
