package logging

import (
	"os"

	jlog "github.com/jeffotoni/log"

	"github.com/jeffotoni/jungle-backend-challenge/internal/config"
)

// New returns the project logger.
// Keep all logger-specific code in this infrastructure package.
func New(cfg config.Config) *jlog.Logger {
	format := jlog.FormatJSON
	if cfg.LogFormat == "text" {
		format = jlog.FormatText
	}
	return jlog.New(jlog.Config{
		Format:      format,
		Writer:      os.Stdout,
		ServiceName: "jungle-backend-challenge",
	})
}
