package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
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
	logger.Info(
		"starting Kumbuka",
		"event", "app_starting",
		"version", version,
		"commit", commit,
	)

	if len(cfg.Overrides) > 0 {
		logger.Info("CLI Overrides", "event", "cli_overrides", "overrides", cfg.Overrides)
	}

	// Bind the process lifetime to operating-system shutdown signals.
	ctx, stop := server.SignalContext(ctx)
	defer stop()

	// Load deployment-owned presentation, encryption, and persistence configuration.
	availableThemes, secretCipher, database, err := loadRunInfrastructure(ctx, cfg, setupLogger)
	if err != nil {
		return err
	}
	defer database.Close()

	// Construct page mutation and collaboration capabilities that other workflows depend on.
	webhooks := appwebhooks.NewWebhooks(database, secretCipher, logger.With("component", "webhooks"), cfg.PublicURL)
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
	notifications := appnotifications.NewNotifications(database)
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

	// Configure browser authentication and construct the plugin runtime.
	browserAuth, renderer, err := createRunRuntime(ctx, cfg, database, secretCipher, logger, setupLogger, version, commit)
	if err != nil {
		return err
	}
	defer closeRenderer(renderer, setupLogger)
	bearerAuth := auth.NewBearer(database)

	// Inject runtime-derived content and icon capabilities into application services.
	iconCatalog := renderer.IconCatalog()
	content := pagecontent.New(renderer)
	navigation.WithIconValidator(iconCatalog)
	mutations.WithIconValidator(iconCatalog).WithContentPreparer(content)
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
	views, err := createRunViews(appFS, logger, setupLogger, version, commit, availableThemes, cfg, secretCipher, iconCatalog, renderer)
	if err != nil {
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
		// Infrastructure.
		Assets:    appFS,
		Views:     views,
		Renderer:  renderer,
		Logger:    serverLogger,
		AccessLog: cfg.AccessLog,
		ReadOnly:  cfg.ReadOnly,

		// Authentication.
		BrowserAuth: browserAuth,
		BearerAuth:  bearerAuth,

		// Shared browser capabilities.
		BrowserContext: browserContext,
		Preferences:    preferences,
		Knowledge:      knowledge,
		Notifications:  notifications,

		// Administration and application management.
		Administration: administration,
		PluginUpdates:  pluginUpdates,
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

		// Page queries.
		Access:        access,
		PageLookup:    pageLookup,
		PageSearch:    pageSearch,
		PageDirectory: pageDirectory,
		PageReports:   pageReports,
		PagePersonal:  pagePersonal,
		PageHistory:   pageHistory,
		PageRender:    pageRender,
		Drafts:        drafts,

		// Page workflows.
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
	}

	handler := httpserver.New(serverConfig)

	return runHTTPServer(ctx, cfg, handler, pluginUpdates, setupLogger)
}

// loadRunInfrastructure loads themes, encryption, and the PostgreSQL store.
func loadRunInfrastructure(ctx context.Context, cfg flags.Config, logger *slog.Logger) ([]themes.Theme, *secrets.Cipher, *postgres.Store, error) {
	availableThemes, err := themes.Load(cfg.ThemeDirectory)
	if err != nil {
		return nil, nil, nil, setupFailure(logger, "load themes", "theme_load_failed", err)
	}
	secretCipher, err := secrets.New(cfg.EncryptionKey)
	if err != nil {
		return nil, nil, nil, setupFailure(logger, "configure application encryption", "application_encryption_failed", err)
	}
	database, err := openRunDatabase(ctx, cfg, logger)
	if err != nil {
		return nil, nil, nil, err
	}
	return availableThemes, secretCipher, database, nil
}

// openRunDatabase applies deployment-level database options and opens the store.
func openRunDatabase(ctx context.Context, cfg flags.Config, logger *slog.Logger) (*postgres.Store, error) {
	var options []postgres.Option
	if cfg.AllowUserRegistrationOverride != nil {
		options = append(options, postgres.WithUserRegistrationOverride(*cfg.AllowUserRegistrationOverride))
	}
	database, err := postgres.Open(ctx, cfg.DatabaseURL, logger, options...)
	if err != nil {
		return nil, setupFailure(logger, "open database", "database_open_failed", err)
	}
	return database, nil
}

// createRunRuntime configures browser authentication and the Markdown/plugin runtime.
func createRunRuntime(ctx context.Context, cfg flags.Config, database *postgres.Store, secretCipher *secrets.Cipher, logger, setupLogger *slog.Logger, version, commit string) (auth.BrowserAuth, *markdown.Renderer, error) {
	browserAuth, err := auth.ConfigureBrowserAuth(ctx, browserAuthConfig(cfg), database)
	if err != nil {
		return auth.BrowserAuth{}, nil, setupFailure(setupLogger, "configure browser auth", "browser_auth_failed", err)
	}
	renderer, err := pluginruntime.NewRenderer(ctx, database, secretCipher, authenticatedPluginRequest, logger, setupLogger, version, commit)
	if err != nil {
		return auth.BrowserAuth{}, nil, err
	}
	return browserAuth, renderer, nil
}

// createRunViews constructs views and enables optional render diagnostics.
func createRunViews(appFS fs.FS, logger, setupLogger *slog.Logger, version, commit string, availableThemes []themes.Theme, cfg flags.Config, secretCipher *secrets.Cipher, iconCatalog *icons.Catalog, renderer *markdown.Renderer) (*webview.Views, error) {
	views, err := webview.New(appFS, logger, version, commit, availableThemes, runtimeinfo.New(cfg, secretCipher.Configured()), iconCatalog)
	if err != nil {
		return nil, setupFailure(setupLogger, "create views", "views_create_failed", err)
	}
	views.WithRenderErrorHandler(httpresponse.InternalServerError)
	if cfg.DebugRenderTimings {
		renderer.EnableRenderTimings(logger.With("component", "markdown"))
		views.EnablePageTimings(logger.With("component", "handler"))
	}
	return views, nil
}

// runHTTPServer starts optional plugin update checks and serves until shutdown.
func runHTTPServer(ctx context.Context, cfg flags.Config, handler http.Handler, pluginUpdates *appplugins.PluginUpdates, logger *slog.Logger) error {
	if cfg.PluginUpdateCheckInterval > 0 {
		go pluginUpdates.Run(ctx)
	}
	if err := server.Run(ctx, cfg.ListenAddress, handler, logger, server.WithMaxHeaderValueCount(100)); err != nil {
		return setupFailure(logger, "run server", "server_run_failed", err)
	}
	return nil
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

// setupFailure records a startup failure and returns the original error.
func setupFailure(logger *slog.Logger, message, event string, err error) error {
	logger.Error(message, "event", event, "error", err)
	return err
}
