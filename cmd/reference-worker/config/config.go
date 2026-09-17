package config

import (
	"time"

	"github.com/jeffotoni/log"

	"github.com/jeffotoni/jungle-backend-challenge/internal/pkg/env"
)

var (
	DATABASE_URL           = env.GetString("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable")
	POLL_INTERVAL          = env.GetDuration("POLL_INTERVAL", time.Second)
	BATCH_SIZE             = env.GetInt("REFERENCE_BATCH_SIZE", 10)
	REFERENCE_TTL          = env.GetDuration("REFERENCE_TTL", 15*time.Minute)
	REFERENCE_MAX_ATTEMPTS = env.GetInt("REFERENCE_MAX_ATTEMPTS", 10)
	RETRY_BASE             = env.GetDuration("REFERENCE_RETRY_BASE", time.Second)
	RETRY_MAX              = env.GetDuration("REFERENCE_RETRY_MAX", time.Minute)
	LOG_LEVEL              = env.GetString("LOG_LEVEL", string(log.DEBUG))
	TRACE_ID               = env.GetString("TRACE_ID", "traceId")
)
