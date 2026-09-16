package fxmodules

import (
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend-challenge/internal/repository/postgres"
)

var Common = fx.Module(
	"common",
	fx.Provide(
		postgres.NewPool,
	),
)
