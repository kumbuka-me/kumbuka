package mirror

import (
	"context"
	"io"

	"github.com/kumbuka-me/kumbuka/internal/logging"
	"github.com/kumbuka-me/kumbuka/internal/store"
)

// Run opens PostgreSQL and writes a complete mirror into the configured output directory.
func Run(ctx context.Context, cfg Config, stdout io.Writer) error {
	if err := validateOutputDir(cfg.OutputDir); err != nil {
		return err
	}

	logger := logging.Setup(cfg.LogFormat, false, stdout).With("component", "mirror")
	database, err := store.Open(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := Export(ctx, database, cfg.OutputDir); err != nil {
		return err
	}

	logger.Info("mirror complete", "event", "mirror_complete", "output", cfg.OutputDir)

	return nil
}
