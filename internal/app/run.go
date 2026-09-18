package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/containeroo/httpgrace/server"
	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/pluginupdate"
	"github.com/kumbuka-me/kumbuka/internal/routes"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/logging"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"github.com/kumbuka-me/kumbuka/plugins"
)

// Run starts the Kumbuka server.
func Run(
	ctx context.Context,
	args []string,
	appFS fs.FS,
	version, commit string,
	stdout, stderr io.Writer,
) error {
	cfg, err := flags.Parse(args, version)
	if err != nil {
		switch {
		case tinyflags.IsHelpRequested(err), tinyflags.IsVersionRequested(err):
			_, _ = fmt.Fprint(stdout, err.Error())
			return nil
		default:
			_, _ = fmt.Fprintln(stderr, err)
			return err
		}
	}

	logger := logging.Setup(cfg.LogFormat, cfg.Debug, stdout)
	setupLogger := logger.With("component", "setup")
	setupLogger.Info(
		"starting Kumbuka",
		"event", "app_starting",
		"version", version,
		"commit", commit,
	)
	if len(cfg.Overrides) > 0 {
		setupLogger.Info(
			"CLI Overrides",
			"event", "cli_overrides",
			"overrides", cfg.Overrides,
		)
	}

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

	routeConfig := newRouteConfig(appFS, cfg, database, secretCipher, logger)

	browserAuth, err := auth.ConfigureBrowserAuth(ctx, browserAuthConfig(cfg), database)
	if err != nil {
		return setupFailure(setupLogger, "configure browser auth", "browser_auth_failed", err)
	}
	routeConfig.BrowserAuth = browserAuth
	routeConfig.BearerAuth = auth.NewBearer(database)

	pluginArchives, err := plugins.Archives()
	if err != nil {
		return setupFailure(setupLogger, "load bundled plugins", "plugin_packages_load_failed", err)
	}

	renderer, err := markdown.NewWithPluginStore(
		ctx,
		database,
		pluginArchives,
		wasm.WithStorage(database),
		wasm.WithPermissions(
			"activity:read",
			"drafts:read",
			"settings:read",
			"settings:write",
			"storage:read",
			"storage:write",
		),
		wasm.WithLogger(logger.With("component", "plugins")),
	)
	if err != nil {
		return setupFailure(setupLogger, "create markdown renderer", "markdown_renderer_failed", err)
	}
	renderer.SetArtifactBuild(version, commit)
	defer closeRenderer(renderer, setupLogger)
	routeConfig.Renderer = renderer

	iconCatalog := renderer.IconCatalog()
	configurePluginAwareServices(&routeConfig, renderer, iconCatalog)
	routeConfig.PluginUpdates = service.NewPluginUpdates(
		pluginupdate.New(pluginupdate.DefaultCatalogURL),
		renderer.PluginManager(),
		database,
		cfg.PluginUpdateCheckInterval,
		logger.With("component", "plugin-updates"),
	)

	views, err := webview.New(
		appFS,
		logger,
		version,
		commit,
		availableThemes,
		runtimeInfo(cfg, secretCipher),
		iconCatalog,
	)
	if err != nil {
		return setupFailure(setupLogger, "create views", "views_create_failed", err)
	}
	routeConfig.Views = views

	if cfg.DebugRenderTimings {
		renderer.EnableRenderTimings(logger.With("component", "markdown"))
		views.EnablePageTimings(logger.With("component", "handler"))
	}

	routeConfig.ViewData = newViewDataLoader(routeConfig)
	router := routes.New(routeConfig)
	if routeConfig.PluginUpdates != nil && cfg.PluginUpdateCheckInterval > 0 {
		go routeConfig.PluginUpdates.Run(ctx)
	}

	if err := server.Run(
		ctx,
		cfg.ListenAddress,
		router,
		setupLogger,
		server.WithMaxHeaderValueCount(100),
	); err != nil {
		return setupFailure(setupLogger, "run server", "server_run_failed", err)
	}

	return nil
}
