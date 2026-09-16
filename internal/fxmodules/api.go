package fxmodules

import (
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jeffotoni/log"
	"go.uber.org/fx"

	apiauth "github.com/jeffotoni/jungle-backend-challenge/cmd/api/auth"
	apiconfig "github.com/jeffotoni/jungle-backend-challenge/cmd/api/config"
	"github.com/jeffotoni/jungle-backend-challenge/cmd/api/handlers"
	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
	appwager "github.com/jeffotoni/jungle-backend-challenge/internal/application/wagering"
	appwallet "github.com/jeffotoni/jungle-backend-challenge/internal/application/wallet"
	"github.com/jeffotoni/jungle-backend-challenge/internal/platform/httpserver"
	"github.com/jeffotoni/jungle-backend-challenge/internal/repository/postgres"
)

var API = fx.Module(
	"api",
	fx.Provide(
		func() *log.Logger {
			return log.New(log.Config{
				Format:      log.FormatJSON,
				Level:       log.Level(apiconfig.LOG_LEVEL),
				ServiceName: "api",
				TraceIDKey:  apiconfig.TRACE_ID,
			})
		},
		func() string {
			return apiconfig.HTTP_ADDR
		},
		func() httpserver.TraceKey {
			return httpserver.TraceKey(apiconfig.TRACE_ID)
		},
		func() bool {
			level := strings.ToUpper(strings.TrimSpace(apiconfig.LOG_LEVEL))
			return level == string(log.DEBUG) || level == string(log.TRACE)
		},
		func(lc fx.Lifecycle) (*pgxpool.Pool, error) { return postgres.NewPool(lc, apiconfig.DATABASE_URL) },
		postgres.NewStore,
		func(store *postgres.Store) ports.WalletStore { return store },
		func(store *postgres.Store) ports.WagerStore { return store },
		postgres.NewTxManager,
		func() *apiauth.Verifier {
			return apiauth.NewVerifier(apiconfig.OIDC_ISSUER, apiconfig.OIDC_AUDIENCE, apiconfig.OIDC_INTERNAL_ROLE)
		},
		appwallet.NewService,
		appwager.NewService,
		handlers.NewRouter,
		httpserver.NewServer,
	),
	fx.Invoke(func(*httpserver.Server) {}),
)
