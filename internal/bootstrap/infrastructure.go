// Package bootstrap composes Kumbuka's process-level runtime dependencies.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	appsystem "github.com/kumbuka-me/kumbuka/internal/application/system"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	appmetrics "github.com/kumbuka-me/kumbuka/internal/metrics"
	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
)

// Infrastructure owns process-level dependencies shared by application services.
type Infrastructure struct {
	// database owns the PostgreSQL-backed persistence store for the process.
	database *postgres.Store
	// cipher encrypts and decrypts persisted application secrets.
	cipher *secrets.Cipher
	// themes contains the embedded and optional deployment-provided themes.
	themes []themes.Theme
	// setupState caches the one-time setup requirement for request-time checks.
	setupState *appsystem.SetupState
	// metrics owns the process-local Prometheus registry and collectors.
	metrics *appmetrics.Registry
}

// NewInfrastructure initializes process-level dependencies in startup order.
func NewInfrastructure(
	ctx context.Context,
	cfg flags.Config,
	logger *slog.Logger,
	version, commit string,
) (*Infrastructure, error) {
	availableThemes, err := themes.Load(themes.Files, cfg.ThemeDirectory)
	if err != nil {
		return nil, fmt.Errorf("load themes: %w", err)
	}

	secretCipher, err := secrets.New(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("configure application encryption: %w", err)
	}

	database, err := postgres.Open(
		ctx,
		cfg.DatabaseURL,
		logger,
		postgres.WithMaxConns(cfg.DatabaseMaxConns),
		postgres.WithMinIdleConns(cfg.DatabaseMinIdleConns),
	)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	setupRequired, err := database.SetupRequired(ctx)
	if err != nil {
		database.Close()
		return nil, fmt.Errorf("read setup state: %w", err)
	}

	metricsRegistry := appmetrics.NewRegistry(!cfg.DisableMetrics, version, commit)
	metricsRegistry.RegisterPostgres(database)

	return &Infrastructure{
		database:   database,
		cipher:     secretCipher,
		themes:     availableThemes,
		setupState: appsystem.NewSetupState(setupRequired),
		metrics:    metricsRegistry,
	}, nil
}

// Close releases infrastructure owned by the process.
func (i *Infrastructure) Close() {
	if i == nil || i.database == nil {
		return
	}
	i.database.Close()
}
