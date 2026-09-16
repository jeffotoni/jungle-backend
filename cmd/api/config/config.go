package config

import (
	"github.com/jeffotoni/log"

	"github.com/jeffotoni/jungle-backend-challenge/internal/pkg/env"
)

var (
	HTTP_ADDR          = env.GetString("HTTP_ADDR", ":8080")
	DATABASE_URL       = env.GetString("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle?sslmode=disable")
	LOG_LEVEL          = env.GetString("LOG_LEVEL", string(log.DEBUG))
	TRACE_ID           = env.GetString("TRACE_ID", "traceId")
	OIDC_ISSUER        = env.GetString("OIDC_ISSUER", "")
	OIDC_AUDIENCE      = env.GetString("OIDC_AUDIENCE", "")
	OIDC_INTERNAL_ROLE = env.GetString("OIDC_INTERNAL_ROLE", "wallet-internal")
)
