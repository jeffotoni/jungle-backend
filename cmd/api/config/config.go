package config

import (
	"time"

	"github.com/jeffotoni/log"

	"github.com/jeffotoni/jungle-backend-challenge/internal/pkg/env"
)

var (
	HTTP_ADDR           = env.GetString("HTTP_ADDR", ":8080")
	DATABASE_URL        = env.GetString("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable")
	LOG_LEVEL           = env.GetString("LOG_LEVEL", string(log.DEBUG))
	TRACE_ID            = env.GetString("TRACE_ID", "traceId")
	OIDC_ISSUER         = env.GetString("OIDC_ISSUER", "http://localhost:8081/realms/jungle")
	OIDC_AUDIENCE       = env.GetString("OIDC_AUDIENCE", "jungle-api")
	OIDC_INTERNAL_ROLE  = env.GetString("OIDC_INTERNAL_ROLE", "wallet-internal")
	AWS_REGION          = env.GetString("AWS_REGION", "us-east-1")
	SQS_ENDPOINT_URL    = env.GetString("SQS_ENDPOINT_URL", "http://localhost:4566")
	SQS_WAGER_QUEUE_URL = env.GetString(
		"SQS_WAGER_QUEUE_URL",
		"http://localhost:4566/000000000000/wager-transactions.fifo",
	)
	SQS_HEALTH_TIMEOUT = env.GetDuration("SQS_HEALTH_TIMEOUT", time.Second)
)
