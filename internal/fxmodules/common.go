package fxmodules

import (
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend-challenge/internal/config"
	"github.com/jeffotoni/jungle-backend-challenge/internal/platform/logging"
	"github.com/jeffotoni/jungle-backend-challenge/internal/platform/postgres"
)

var Common = fx.Module(
	"common",
	fx.Provide(
		config.Load,
		logging.New,
		postgres.NewPool,
	),
)
