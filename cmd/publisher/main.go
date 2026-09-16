package main

import (
	"context"

	"github.com/jeffotoni/log"
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend-challenge/cmd/publisher/config"
	"github.com/jeffotoni/jungle-backend-challenge/internal/fxmodules"
)

type Publisher struct{}

func NewPublisher(lc fx.Lifecycle) *Publisher {
	p := &Publisher{}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// TODO: start transactional outbox publishing loop.
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// TODO: cancel loop and finish safely.
			return nil
		},
	})
	return p
}

func main() {
	fx.New(
		fxmodules.Common,
		fx.NopLogger,
		fx.Provide(
			func() string {
				return config.DATABASE_URL
			},
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
