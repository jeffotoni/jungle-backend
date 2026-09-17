package config

import (
	"time"

	"github.com/jeffotoni/log"

	"github.com/jeffotoni/jungle-backend-challenge/internal/pkg/env"
)

var (
	DATABASE_URL        = env.GetString("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable")
	AWS_REGION          = env.GetString("AWS_REGION", "us-east-1")
	SQS_ENDPOINT_URL    = env.GetString("SQS_ENDPOINT_URL", "http://localhost:4566")
	SQS_WAGER_QUEUE_URL = env.GetString("SQS_WAGER_QUEUE_URL", "http://localhost:4566/000000000000/wager-transactions.fifo")
	VISIBILITY_TIMEOUT  = env.GetDuration("VISIBILITY_TIMEOUT", 30*time.Second)
	SQS_WAIT_TIME       = env.GetDuration("SQS_WAIT_TIME", 20*time.Second)
	SQS_MAX_MESSAGES    = env.GetInt("SQS_MAX_MESSAGES", 10)
	RETRY_BASE          = env.GetDuration("RETRY_BASE", time.Second)
	RETRY_MAX           = env.GetDuration("RETRY_MAX", time.Minute)
	CONSUMER_NAME       = env.GetString("CONSUMER_NAME", "wager-consumer")
	LOG_LEVEL           = env.GetString("LOG_LEVEL", string(log.DEBUG))
	TRACE_ID            = env.GetString("TRACE_ID", "traceId")
)
