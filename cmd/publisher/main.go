package main

import (
	"context"

	"go.uber.org/fx"

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
		fx.Provide(NewPublisher),
		fx.Invoke(func(*Publisher) {}),
	).Run()
}
