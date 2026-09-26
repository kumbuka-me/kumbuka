package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"time"

	"github.com/containeroo/httpgrace/server"
	"github.com/containeroo/tinyflags"
	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	appadministration "github.com/kumbuka-me/kumbuka/internal/application/administration"
	appgroups "github.com/kumbuka-me/kumbuka/internal/application/groups"
	appmedia "github.com/kumbuka-me/kumbuka/internal/application/media"
	appnavigation "github.com/kumbuka-me/kumbuka/internal/application/navigation"
	appnotifications "github.com/kumbuka-me/kumbuka/internal/application/notifications"
	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	appplugins "github.com/kumbuka-me/kumbuka/internal/application/plugins"
	apppreferences "github.com/kumbuka-me/kumbuka/internal/application/preferences"
	apprecyclebin "github.com/kumbuka-me/kumbuka/internal/application/recyclebin"
	appsearch "github.com/kumbuka-me/kumbuka/internal/application/search"
	appsettings "github.com/kumbuka-me/kumbuka/internal/application/settings"
	appsystem "github.com/kumbuka-me/kumbuka/internal/application/system"
	apptemplates "github.com/kumbuka-me/kumbuka/internal/application/templates"
	apptokens "github.com/kumbuka-me/kumbuka/internal/application/tokens"
	appusers "github.com/kumbuka-me/kumbuka/internal/application/users"
	"github.com/kumbuka-me/kumbuka/internal/application/viewer"
	appwebhooks "github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/internal/credential"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	httpserver "github.com/kumbuka-me/kumbuka/internal/http/server"
	appmetrics "github.com/kumbuka-me/kumbuka/internal/metrics"
	"github.com/kumbuka-me/kumbuka/internal/pagecontent"
	"github.com/kumbuka-me/kumbuka/internal/pluginruntime"
	"github.com/kumbuka-me/kumbuka/internal/pluginupdate"
	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/kumbuka-me/kumbuka/internal/runtimeinfo"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/kumbuka-me/kumbuka/pkg/logging"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
)

const rendererShutdownTimeout = 10 * time.Second

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

	metricsRegistry := appmetrics.NewRegistry(!cfg.DisableMetrics, version, commit)

	if len(cfg.Overrides) > 0 {
		setupLogger.Info("CLI Overrides", "event", "cli_overrides", "overrides", cfg.Overrides)
	}

	// Bind the process lifetime to operating-system shutdown signals.
	ctx, stop := server.SignalContext(ctx)
	defer stop()

	availableThemes, err := themes.Load(cfg.ThemeDirectory)
	if err != nil {
		setupLogger.Error("load themes", "event", "theme_load_failed", "error", err)
		return err
	}

	secretCipher, err := secrets.New(cfg.EncryptionKey)
	if err != nil {
		setupLogger.Error("configure application encryption", "event", "application_encryption_failed", "error", err)
		return err

	}

	databaseOptions := []postgres.Option{
		postgres.WithMaxConns(cfg.DatabaseMaxConns),
		postgres.WithMinIdleConns(cfg.DatabaseMinIdleConns),
	}
	if cfg.AllowUserRegistrationOverride != nil {
		databaseOptions = append(databaseOptions, postgres.WithUserRegistrationOverride(*cfg.AllowUserRegistrationOverride))
	}

	database, err := postgres.Open(ctx, cfg.DatabaseURL, logger, databaseOptions...)
	if err != nil {
		setupLogger.Error("open database", "event", "database_open_failed", "error", err)
		return errors.New("database_open_failed")
	}

	defer database.Close()
	metricsRegistry.RegisterPostgres(database)

	// Construct page mutation and collaboration capabilities that other workflows depend on.
	webhooks := appwebhooks.NewWebhooks(database, secretCipher, logger.With("component", "webhooks"), cfg.PublicURL).WithUserDirectory(database)
	access := appaccess.NewAccess(database)
	mutations := apppages.NewMutations(database, access, database, logger, webhooks)
	presence := apppages.NewPresence(database, access)
	discussions := apppages.NewDiscussions(database, access, nil, database, logger, webhooks)
	reviews := apppages.NewReviews(database, access, database, logger, webhooks)
	reviewDiscussions := apppages.NewReviewDiscussions(database, access, reviews, nil, database, logger, webhooks)
	bulk := apppages.NewBulk(database, mutations, database, logger, webhooks)

	// Construct the remaining application capabilities around their narrow repository ports.
	administration := appadministration.NewAdministration(database)
	pageLookup := apppages.NewLookup(database, access)
	pageSearch := apppages.NewSearch(database, access)
	pageDirectory := apppages.NewDirectory(database, access)
	pageReports := apppages.NewReports(database, access)
	pagePersonal := apppages.NewPersonal(database, access)
	pageHistory := apppages.NewHistory(database, access)
	pageRender := apppages.NewRenderArtifacts(database)
	drafts := apppages.NewDrafts(database)
	groups := appgroups.NewGroups(database)
	knowledge := appsearch.NewKnowledge(database, access)
	notifications := appnotifications.NewNotifications(database, webhooks).WithLogger(logger.With("component", "notifications"))
	media := appmedia.NewMedia(database)
	navigation := appnavigation.NewNavigation(database, access)
	preferences := apppreferences.NewPreferences(database)
	recycleBin := apprecyclebin.NewRecycleBin(database)
	settings := appsettings.NewSettings(database, secretCipher).WithLogger(logger.With("component", "settings"))
	system := appsystem.NewSystem(database).WithLogger(logger.With("component", "system"))
	templates := apptemplates.NewTemplates(database)
	tokens := apptokens.NewTokens(database)
	users := appusers.NewUsers(database, credential.Passwords{}).WithLogger(logger.With("component", "users"))

	// Compose higher-level page workflows from the capabilities they coordinate.
	serverLogger := logger.With("component", "server")
	home := apppages.NewHomeQuery(database, drafts, access)
	editor := apppages.NewEditor(pageLookup, groups, templates)
	editorSave := apppages.NewEditorSave(mutations, drafts, templates, serverLogger)
	viewPage := apppages.NewView(database, access, reviews, serverLogger)

	// Configure browser authentication.
	browserAuth, err := auth.ConfigureBrowserAuth(ctx, browserAuthConfig(cfg), database)
	if err != nil {
		setupLogger.Error("configure browser auth", "event", "browser_auth_failed", "error", err)
		return err
	}

	// Construct the plugin runtime.
	renderer, err := pluginruntime.NewRenderer(
		ctx,
		database,
		secretCipher,
		authenticatedPluginRequest,
		metricsRegistry,
		logger.With("component", "plugins"),
		version,
		commit,
	)
	if err != nil {
		setupLogger.Error("create plugin runtime", "event", "plugin_runtime_failed", "error", err)
		return err
	}

	defer closeRenderer(renderer, setupLogger)
	metricsRegistry.RegisterPluginProvider(renderer.PluginManager())
	bearerAuth := auth.NewBearer(database)

	// Inject runtime-derived content and icon capabilities into application services.
	iconCatalog := renderer.IconCatalog()
	content := pagecontent.New(renderer)
	navigation.WithIconValidator(iconCatalog)
	contentChanges := pluginruntime.NewContentChanges(
		database,
		renderer.PluginManager(),
		notifications,
		logger.With("component", "plugin-content-changes"),
	)
	mutations.WithIconValidator(iconCatalog).
		WithContentPreparer(content).
		WithContentChangeSink(contentChanges)
	discussions.WithContentPreparer(content)
	reviewDiscussions.WithContentPreparer(content)
	settings.WithIconValidator(iconCatalog)
	templates.WithIconValidator(iconCatalog)

	// Construct the optional background plugin-update capability.
	pluginUpdates := appplugins.NewPluginUpdates(
		pluginupdate.New(pluginupdate.DefaultCatalogURL),
		renderer.PluginManager(),
		database,
		cfg.PluginUpdateCheckInterval,
		logger.With("component", "plugin-updates"),
	)

	// Construct and configure the passive HTML presentation adapter.
	views, err := createRunViews(appFS, logger, version, commit, availableThemes, cfg, secretCipher, iconCatalog, renderer)
	if err != nil {
		setupLogger.Error("create views", "event", "views_create_failed", "error", err)
		return err
	}

	// Compose the shared authenticated browser context used by presentation endpoints.
	browserContext := endpoint.NewBrowserContext(viewer.New(
		preferences,
		navigation,
		database,
		settings,
		knowledge,
		notifications,
		access,
	), renderer)

	// Hand the completed application graph to the HTTP adapter for route construction.
	serverConfig := httpserver.Config{
		InfrastructureConfig: httpserver.InfrastructureConfig{
			Assets:         appFS,
			Views:          views,
			Renderer:       renderer,
			Logger:         serverLogger,
			AccessLog:      cfg.AccessLog,
			ReadOnly:       cfg.ReadOnly,
			MetricsEnabled: !cfg.DisableMetrics,
			Metrics:        metricsRegistry,
		},

		AuthenticationConfig: httpserver.AuthenticationConfig{
			BrowserAuth: browserAuth,
			BearerAuth:  bearerAuth,
		},

		BrowserConfig: httpserver.BrowserConfig{
			BrowserContext: browserContext,
			Preferences:    preferences,
			Knowledge:      knowledge,
			Notifications:  notifications,
		},

		AdministrationConfig: httpserver.AdministrationConfig{
			Administration: administration,
			PluginAdmin:    appplugins.NewAdmin(renderer.PluginManager(), pluginUpdates),
			Groups:         groups,
			Settings:       settings,
			System:         system,
			Templates:      templates,
			Tokens:         tokens,
			Users:          users,
			Webhooks:       webhooks,
			Media:          media,
			Navigation:     navigation,
			RecycleBin:     recycleBin,
		},

		PageQueryConfig: httpserver.PageQueryConfig{
			Access:        access,
			PageLookup:    pageLookup,
			PageSearch:    pageSearch,
			PageDirectory: pageDirectory,
			PageReports:   pageReports,
			PagePersonal:  pagePersonal,
			PageHistory:   pageHistory,
			PageRender:    pageRender,
			Drafts:        drafts,
		},

		PageWorkflowConfig: httpserver.PageWorkflowConfig{
			PageMutations:         mutations,
			PagePresence:          presence,
			PageDiscussions:       discussions,
			PageReviews:           reviews,
			PageReviewDiscussions: reviewDiscussions,
			PageBulk:              bulk,
			Home:                  home,
			Editor:                editor,
			EditorSave:            editorSave,
			ViewPage:              viewPage,
		},
	}

	go contentChanges.Run(ctx)
	if cfg.PluginUpdateCheckInterval > 0 {
		go pluginUpdates.Run(ctx)
	}

	handler := httpserver.New(serverConfig)
	if err := server.Run(ctx, cfg.ListenAddress, handler, logger, server.WithMaxHeaderValueCount(100)); err != nil {
		setupLogger.Error("run server", "event", "server_run_failed", "error", err)
		return err
	}

	return nil
}

// createRunViews constructs views and enables optional render diagnostics.
func createRunViews(
	appFS fs.FS,
	logger *slog.Logger,
	version, commit string,
	availableThemes []themes.Theme,
	cfg flags.Config,
	secretCipher *secrets.Cipher,
	iconCatalog *icons.Catalog,
	renderer *markdown.Renderer,
) (*webview.Views, error) {
	views, err := webview.New(
		appFS,
		logger,
		version,
		commit,
		availableThemes,
		runtimeinfo.New(cfg, secretCipher.Configured()),
		iconCatalog,
	)
	if err != nil {
		return nil, err
	}
	views.WithRenderErrorHandler(httpresponse.InternalServerError)
	if cfg.DebugRenderTimings {
		renderer.EnableRenderTimings(logger.With("component", "markdown"))
		views.EnablePageTimings(logger.With("component", "handler"))
	}
	return views, nil
}

// browserAuthConfig maps deployment configuration onto the authentication boundary.
func browserAuthConfig(cfg flags.Config) auth.BrowserConfig {
	return auth.BrowserConfig{
		ModeOverride: cfg.AuthModeOverride,
		TrustedProxy: auth.TrustedProxyHeaders{
			Username:    cfg.TrustedUsernameHeaders,
			Email:       cfg.TrustedEmailHeaders,
			DisplayName: cfg.TrustedDisplayNameHeaders,
			Groups:      cfg.TrustedGroupHeaders,
			AdminGroup:  cfg.TrustedAdminGroup,
		},
		OIDC: auth.OIDCConfig{
			ClientID:      cfg.OIDCClientID,
			ClientSecret:  cfg.OIDCClientSecret,
			Issuer:        cfg.OIDCIssuer,
			SessionSecret: cfg.OIDCSessionSecret,
			PublicURL:     cfg.PublicURL,
			GroupClaim:    cfg.OIDCGroupClaim,
			AdminGroup:    cfg.OIDCAdminGroup,
		},
		LocalLoginEnabled: cfg.LocalLogin,
	}
}

// authenticatedPluginRequest reports whether the current plugin invocation belongs to an authenticated user.
func authenticatedPluginRequest(ctx context.Context) bool {
	user, ok := auth.ContextUser(ctx)
	return ok && user.ID > 0
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
