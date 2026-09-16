package config

import (
	"time"

	"github.com/jeffotoni/log"

	"github.com/jeffotoni/jungle-backend-challenge/internal/pkg/env"
)

var (
	DATABASE_URL        = env.GetString("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable")
	AWS_REGION          = env.GetString("AWS_REGION", "us-east-1")
	SQS_WAGER_QUEUE_URL = env.GetString("SQS_WAGER_QUEUE_URL", "")
	VISIBILITY_TIMEOUT  = env.GetDuration("VISIBILITY_TIMEOUT", 30*time.Second)
	LOG_LEVEL           = env.GetString("LOG_LEVEL", string(log.DEBUG))
	TRACE_ID            = env.GetString("TRACE_ID", "traceId")
)
