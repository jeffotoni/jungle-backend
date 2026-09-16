package main

import (
	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend-challenge/internal/fxmodules"
)

func main() {
	fx.New(
		fxmodules.Common,
		fxmodules.API,
	).Run()
}
