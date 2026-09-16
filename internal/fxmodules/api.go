package fxmodules

import (
	"net/http"

	"github.com/jeffotoni/quick"
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend-challenge/internal/platform/httpserver"
)

var API = fx.Module(
	"api",
	fx.Provide(
		httpserver.NewRouter,
		httpserver.NewServer,
	),
	fx.Invoke(func(*http.Server, *quick.Quick) {}),
)
