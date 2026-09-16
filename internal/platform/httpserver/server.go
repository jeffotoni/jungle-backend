package httpserver

import (
	"context"
	"fmt"

	jlog "github.com/jeffotoni/log"
	"github.com/jeffotoni/quick"
	"go.uber.org/fx"
)

func NewRouter() *quick.Quick {
	return quick.New()
}

type Server struct {
	router   *quick.Quick
	address  string
	logger   *jlog.Logger
	shutdown func()
}

type TraceKey string

func NewServer(
	lc fx.Lifecycle,
	address string,
	router *quick.Quick,
	logger *jlog.Logger,
	traceKey TraceKey,
	captureDetails bool,
) *Server {
	srv := &Server{
		router:  router,
		address: address,
		logger:  logger,
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			handler := HTTPMiddleware(
				srv.logger,
				string(traceKey),
				captureDetails,
			)(srv.router.Handler())
			_, shutdown, err := srv.router.ListenWithShutdown(srv.address, handler)
			if err != nil {
				return fmt.Errorf("start HTTP server: %w", err)
			}
			srv.shutdown = shutdown
			_ = srv.logger.Info().
				Str("address", srv.address).
				Msg("api started").
				Send()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			if srv.shutdown == nil {
				return nil
			}
			srv.shutdown()
			srv.shutdown = nil
			_ = srv.logger.Info().Msg("api stopped").Send()
			return nil
		},
	})

	return srv
}
