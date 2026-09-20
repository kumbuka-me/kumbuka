package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
)

const rendererShutdownTimeout = 10 * time.Second

// openDatabase applies deployment-managed store options before opening PostgreSQL.
func openDatabase(ctx context.Context, cfg flags.Config, logger *slog.Logger) (*postgres.Store, error) {
	options := make([]postgres.Option, 0, 1)
	if cfg.AllowUserRegistrationOverride != nil {
		options = append(options, postgres.WithUserRegistrationOverride(*cfg.AllowUserRegistrationOverride))
	}

	database, err := postgres.Open(ctx, cfg.DatabaseURL, logger, options...)
	if err != nil {
		return nil, setupFailure(logger, "open database", "database_open_failed", err)
	}

	return database, nil
}

// closeRenderer gives plugin shutdown a bounded fresh context after server cancellation.
func closeRenderer(renderer *markdown.Renderer, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), rendererShutdownTimeout)
	defer cancel()

	if err := renderer.Close(ctx); err != nil {
		logger.Error(
			"close markdown renderer",
			"event", "markdown_renderer_close_failed",
			"error", err,
		)
	}
}

// setupFailure records a startup failure and returns the original error.
func setupFailure(logger *slog.Logger, message, event string, err error) error {
	logger.Error(message, "event", event, "error", err)
	return err
}
