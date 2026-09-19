package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"

	"github.com/containeroo/httpgrace/server"
	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/logging"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
)

// Run starts the Kumbuka server.
func Run(
	ctx context.Context,
	args []string,
	appFS fs.FS,
	version, commit string,
	stdout, stderr io.Writer,
) error {
	cfg, exit, err := parseArguments(args, version, stdout, stderr)
	if err != nil || exit {
		return err
	}

	logger := logging.Setup(cfg.LogFormat, cfg.Debug, stdout)
	setupLogger := logger.With("component", "setup")
	logStartup(setupLogger, cfg, version, commit)

	// Install signal cancellation before startup performs network or database work.
	ctx, stop := server.SignalContext(ctx)
	defer stop()

	availableThemes, err := themes.Load(cfg.ThemeDirectory)
	if err != nil {
		return setupFailure(setupLogger, "load themes", "theme_load_failed", err)
	}

	secretCipher, err := secrets.New(cfg.EncryptionKey)
	if err != nil {
		return setupFailure(setupLogger, "configure application encryption", "application_encryption_failed", err)
	}

	database, err := openDatabase(ctx, cfg, setupLogger)
	if err != nil {
		return err
	}
	defer database.Close()

	runtime, err := newApplicationRuntime(
		ctx,
		appFS,
		cfg,
		database,
		secretCipher,
		logger,
		setupLogger,
		version,
		commit,
		availableThemes,
	)
	if err != nil {
		return err
	}
	defer closeRenderer(runtime.renderer, setupLogger)

	runtime.startBackgroundTasks(ctx, cfg)
	if err := server.Run(
		ctx,
		cfg.ListenAddress,
		runtime.handler,
		setupLogger,
		server.WithMaxHeaderValueCount(100),
	); err != nil {
		return setupFailure(setupLogger, "run server", "server_run_failed", err)
	}

	return nil
}

// parseArguments parses deployment configuration and handles non-error help or version exits.
func parseArguments(args []string, version string, stdout, stderr io.Writer) (flags.Config, bool, error) {
	cfg, err := flags.Parse(args, version)
	if err == nil {
		return cfg, false, nil
	}

	if tinyflags.IsHelpRequested(err) || tinyflags.IsVersionRequested(err) {
		_, _ = fmt.Fprint(stdout, err.Error())
		return flags.Config{}, true, nil
	}

	_, _ = fmt.Fprintln(stderr, err)
	return flags.Config{}, false, err
}

// logStartup records application identity and explicit deployment overrides.
func logStartup(logger *slog.Logger, cfg flags.Config, version, commit string) {
	logger.Info(
		"starting Kumbuka",
		"event", "app_starting",
		"version", version,
		"commit", commit,
	)

	if len(cfg.Overrides) == 0 {
		return
	}

	logger.Info(
		"CLI Overrides",
		"event", "cli_overrides",
		"overrides", cfg.Overrides,
	)
}
