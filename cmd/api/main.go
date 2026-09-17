package main

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/fx"

	"github.com/jeffotoni/jungle-backend/internal/fxmodules"
)

func main() {
	app := fx.New(
		fxmodules.API,
		fx.NopLogger,
	)
	if err := app.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "api startup error:", err)
		os.Exit(1)
	}
	if err := app.Start(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "api startup error:", err)
		_ = app.Stop(context.Background())
		os.Exit(1)
	}
	defer app.Stop(context.Background())
	<-app.Done()
}
