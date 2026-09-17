package main

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/jeffotoni/log"
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend/cmd/publisher/config"
	"github.com/jeffotoni/jungle-backend/internal/application/ports"
	"github.com/jeffotoni/jungle-backend/internal/repository/postgres"
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
			func(store *postgres.Store) ports.OutboxStore { return store },
			newSQSClient,
			func() *log.Logger {
				return log.New(log.Config{
					Format:      log.FormatJSON,
					Level:       log.Level(config.LOG_LEVEL),
					ServiceName: "publisher",
					TraceIDKey:  config.TRACE_ID,
				})
			},
		),
		fx.Provide(NewPublisher),
		fx.Invoke(func(*Publisher) {}),
	).Run()
}
