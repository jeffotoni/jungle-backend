package config

import (
	"time"

	"github.com/jeffotoni/log"

	"github.com/jeffotoni/jungle-backend/internal/pkg/env"
)

var (
	DATABASE_URL         = env.GetString("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable")
	AWS_REGION           = env.GetString("AWS_REGION", "us-east-1")
	SQS_ENDPOINT_URL     = env.GetString("SQS_ENDPOINT_URL", "http://localhost:4566")
	SQS_EVENTS_QUEUE_URL = env.GetString("SQS_EVENTS_QUEUE_URL", "http://localhost:4566/000000000000/jungle-events")
	OUTBOX_BATCH_SIZE    = env.GetInt("OUTBOX_BATCH_SIZE", 100)
	PUBLISHER_NAME       = env.GetString("PUBLISHER_NAME", "outbox-publisher")
	POLL_INTERVAL        = env.GetDuration("POLL_INTERVAL", time.Second)
	PUBLISHER_LEASE      = env.GetDuration("PUBLISHER_LEASE", 30*time.Second)
	RETRY_BASE           = env.GetDuration("RETRY_BASE", time.Second)
	RETRY_MAX            = env.GetDuration("RETRY_MAX", time.Minute)
	SQS_EVENT_GROUP_ID   = env.GetString("SQS_EVENT_GROUP_ID", "outbox-events")
	LOG_LEVEL            = env.GetString("LOG_LEVEL", string(log.DEBUG))
	TRACE_ID             = env.GetString("TRACE_ID", "traceId")
)
