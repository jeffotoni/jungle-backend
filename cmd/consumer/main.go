package main

import (
	"context"

	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend-challenge/internal/fxmodules"
)

type Consumer struct{}

func NewConsumer(lc fx.Lifecycle) *Consumer {
	c := &Consumer{}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// TODO: start SQS long-poll loop.
			return nil
		},
		OnStop: func(ctx context.Context) error {
			// TODO: cancel polling and finish safely.
			return nil
		},
	})
	return c
}

func main() {
	fx.New(
		fxmodules.Common,
		fx.Provide(NewConsumer),
		fx.Invoke(func(*Consumer) {}),
	).Run()
}
