package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/jeffotoni/quick"
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend-challenge/internal/config"
)

func NewRouter() *quick.Quick {
	app := quick.New()

	app.Get("/health", func(c *quick.Ctx) error {
		return c.Status(quick.StatusOK).SendString("ok")
	})

	return app
}

func NewServer(lc fx.Lifecycle, cfg config.Config, router *quick.Quick) *http.Server {
	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: router.Handler(),
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				err := srv.ListenAndServe()
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					// lifecycle cannot surface async ListenAndServe errors;
					// structured logging/health monitoring will be wired in next phase.
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})

	return srv
}
