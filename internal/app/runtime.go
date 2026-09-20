package app

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"

	appplugins "github.com/kumbuka-me/kumbuka/internal/application/plugins"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	httpserver "github.com/kumbuka-me/kumbuka/internal/http/server"
	"github.com/kumbuka-me/kumbuka/internal/pluginupdate"
	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"github.com/kumbuka-me/kumbuka/plugins"
)

// applicationRuntime contains the request handler and runtime-owned background components.
type applicationRuntime struct {
	// handler is the fully configured HTTP application.
	handler http.Handler
	// renderer owns plugin runtime resources that must be closed during shutdown.
	renderer *markdown.Renderer
	// pluginUpdates performs optional scheduled first-party catalog refreshes.
	pluginUpdates *appplugins.PluginUpdates
}

// newApplicationRuntime composes runtime-bound authentication, plugins, views, and routes.
func newApplicationRuntime(
	ctx context.Context,
	appFS fs.FS,
	cfg flags.Config,
	database *postgres.Store,
	secretCipher *secrets.Cipher,
	logger, setupLogger *slog.Logger,
	version, commit string,
	availableThemes []themes.Theme,
) (applicationRuntime, error) {
	routeConfig := newRouteConfig(appFS, cfg, database, secretCipher, logger)
	if err := configureAuthentication(ctx, &routeConfig, cfg, database, setupLogger); err != nil {
		return applicationRuntime{}, err
	}

	renderer, err := newRenderer(ctx, database, secretCipher, logger, setupLogger, version, commit)
	if err != nil {
		return applicationRuntime{}, err
	}
	routeConfig.Renderer = renderer

	iconCatalog := renderer.IconCatalog()
	configurePluginAwareServices(&routeConfig, renderer, iconCatalog)
	routeConfig.PluginUpdates = newPluginUpdateService(cfg, renderer, database, logger)

	views, err := newViews(appFS, cfg, secretCipher, iconCatalog, logger, setupLogger, version, commit, availableThemes)
	if err != nil {
		closeRenderer(renderer, setupLogger)
		return applicationRuntime{}, err
	}
	routeConfig.Views = views

	configureRenderTimings(cfg, renderer, views, logger)
	routeConfig.BrowserContext = newBrowserContext(routeConfig, database)

	handler := httpserver.New(routeConfig)

	return applicationRuntime{
		handler:       handler,
		renderer:      renderer,
		pluginUpdates: routeConfig.PluginUpdates,
	}, nil
}

// configureAuthentication attaches browser and bearer authentication to the route configuration.
func configureAuthentication(
	ctx context.Context,
	config *httpserver.Config,
	cfg flags.Config,
	database *postgres.Store,
	setupLogger *slog.Logger,
) error {
	browserAuth, err := auth.ConfigureBrowserAuth(ctx, browserAuthConfig(cfg), database)
	if err != nil {
		return setupFailure(setupLogger, "configure browser auth", "browser_auth_failed", err)
	}

	config.BrowserAuth = browserAuth
	config.BearerAuth = auth.NewBearer(database)
	return nil
}

// newRenderer creates the Markdown renderer and configures its plugin runtime resources.
func newRenderer(
	ctx context.Context,
	database *postgres.Store,
	secretCipher *secrets.Cipher,
	logger, setupLogger *slog.Logger,
	version, commit string,
) (*markdown.Renderer, error) {
	pluginArchives, err := plugins.Archives()
	if err != nil {
		return nil, setupFailure(setupLogger, "load bundled plugins", "plugin_packages_load_failed", err)
	}

	renderer, err := markdown.NewWithPluginStore(
		ctx,
		database,
		pluginArchives,
		wasm.WithStorage(database),
		wasm.WithSecretCodec(secretCipher),
		wasm.WithHTTPAuthorizer(authenticatedPluginRequest),
		wasm.WithPermissions(
			"network:http",
			"network:private",
			"network:insecure-tls",
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
		return nil, setupFailure(setupLogger, "create markdown renderer", "markdown_renderer_failed", err)
	}

	renderer.PluginManager().SetSecretCodec(secretCipher)
	renderer.SetArtifactBuild(version, commit)
	return renderer, nil
}

// authenticatedPluginRequest reports whether the current plugin invocation belongs to an authenticated user.
func authenticatedPluginRequest(ctx context.Context) bool {
	user, ok := auth.ContextUser(ctx)
	return ok && user.ID > 0
}

// newPluginUpdateService constructs the optional background plugin update coordinator.
func newPluginUpdateService(
	cfg flags.Config,
	renderer *markdown.Renderer,
	database *postgres.Store,
	logger *slog.Logger,
) *appplugins.PluginUpdates {
	return appplugins.NewPluginUpdates(
		pluginupdate.New(pluginupdate.DefaultCatalogURL),
		renderer.PluginManager(),
		database,
		cfg.PluginUpdateCheckInterval,
		logger.With("component", "plugin-updates"),
	)
}

// newViews creates the HTML view layer from application and deployment configuration.
func newViews(
	appFS fs.FS,
	cfg flags.Config,
	secretCipher *secrets.Cipher,
	iconCatalog *icons.Catalog,
	logger, setupLogger *slog.Logger,
	version, commit string,
	availableThemes []themes.Theme,
) (*webview.Views, error) {
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
		return nil, setupFailure(setupLogger, "create views", "views_create_failed", err)
	}

	views.WithRenderErrorHandler(httpresponse.InternalServerError)
	return views, nil
}

// configureRenderTimings enables optional renderer and handler timing diagnostics.
func configureRenderTimings(cfg flags.Config, renderer *markdown.Renderer, views *webview.Views, logger *slog.Logger) {
	if !cfg.DebugRenderTimings {
		return
	}

	renderer.EnableRenderTimings(logger.With("component", "markdown"))
	views.EnablePageTimings(logger.With("component", "handler"))
}

// startBackgroundTasks launches runtime services that live until the application context is canceled.
func (r applicationRuntime) startBackgroundTasks(ctx context.Context, cfg flags.Config) {
	if r.pluginUpdates == nil || cfg.PluginUpdateCheckInterval <= 0 {
		return
	}

	go r.pluginUpdates.Run(ctx)
}
