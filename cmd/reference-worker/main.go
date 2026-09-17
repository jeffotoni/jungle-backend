package main

import (
	"github.com/jeffotoni/log"
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend/cmd/reference-worker/config"
	"github.com/jeffotoni/jungle-backend/internal/application/ports"
	appwager "github.com/jeffotoni/jungle-backend/internal/application/wagering"
	"github.com/jeffotoni/jungle-backend/internal/repository/postgres"
)

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
			func(store *postgres.Store) ports.PendingReferenceStore { return store },
			appwager.NewService,
			func() *log.Logger {
				return log.New(log.Config{
					Format:      log.FormatJSON,
					Level:       log.Level(config.LOG_LEVEL),
					ServiceName: "reference-worker",
					TraceIDKey:  config.TRACE_ID,
				})
			},
		),
		fx.Provide(NewReferenceWorker),
		fx.Invoke(func(*ReferenceWorker) {}),
	).Run()
}
