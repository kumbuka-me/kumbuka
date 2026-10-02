package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/containeroo/httpgrace/server"
	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/bootstrap"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	httpserver "github.com/kumbuka-me/kumbuka/internal/http/server"
	"github.com/kumbuka-me/kumbuka/pkg/logging"
)

// Run composes and runs the Kumbuka process.
func Run(
	ctx context.Context,
	args []string,
	appFS fs.FS,
	version, commit string,
	stdout, stderr io.Writer,
) error {
	// Parse deployment configuration before constructing runtime dependencies.
	cfg, err := flags.Parse(args, version)
	if err != nil {
		if tinyflags.IsHelpRequested(err) || tinyflags.IsVersionRequested(err) {
			fmt.Fprint(stdout, err.Error()) // nolint:errcheck
			return nil
		}
		fmt.Fprintln(stderr, err) // nolint:errcheck
		return err
	}

	// Configure process logging and record the effective application identity.
	logger := logging.Setup(cfg.LogFormat, cfg.Debug, stdout)
	setupLogger := logger.With("component", "setup")
	setupLogger.Info(
		"starting Kumbuka",
		"event", "app_starting",
		"version", version,
		"commit", commit,
	)
	if len(cfg.Overrides) > 0 {
		setupLogger.Info("CLI Overrides", "event", "cli_overrides", "overrides", cfg.Overrides.Values())
	}

	// Bind the process lifetime to operating-system shutdown signals.
	ctx, stop := server.SignalContext(ctx)
	defer stop()

	// Construct process infrastructure and own its lifetime here.
	infrastructure, err := bootstrap.NewInfrastructure(ctx, cfg, logger, version, commit)
	if err != nil {
		setupLogger.Error("initialize infrastructure", "event", "infrastructure_init_failed", "error", err)
		return err
	}
	defer infrastructure.Close()

	// Compose application capabilities around the initialized infrastructure.
	application, err := bootstrap.NewApplication(ctx, cfg, infrastructure, logger, version, commit)
	if err != nil {
		setupLogger.Error("initialize application", "event", "application_init_failed", "error", err)
		return err
	}
	defer application.Close(setupLogger)

	// Build the passive HTTP adapter after the application graph is complete.
	serverConfig, err := bootstrap.NewHTTPConfig(appFS, cfg, infrastructure, application, logger, version, commit)
	if err != nil {
		setupLogger.Error("initialize HTTP server", "event", "http_init_failed", "error", err)
		return err
	}

	// Start background application work only after every dependency is configured.
	application.Start(ctx)

	// Construct both HTTP handlers and run their listeners until shutdown.
	handlers := httpserver.New(serverConfig)
	if err := httpserver.Run(ctx, cfg.ApplicationListenAddress, cfg.ManagementListenAddress, handlers, logger); err != nil {
		setupLogger.Error("run server", "event", "server_run_failed", "error", err)
		return err
	}

	return nil
}
