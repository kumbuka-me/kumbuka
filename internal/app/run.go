package app

import (
	"context"
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
	"github.com/kumbuka-me/kumbuka/internal/pagecontent"
	"github.com/kumbuka-me/kumbuka/internal/pluginruntime"
	"github.com/kumbuka-me/kumbuka/internal/pluginupdate"
	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/webview"
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
	cfg, exit, err := parseArguments(args, version, stdout, stderr)
	if err != nil || exit {
		return err
	}

	logger := logging.Setup(cfg.LogFormat, cfg.Debug, stdout)
	setupLogger := logger.With("component", "setup")
	logStartup(setupLogger, cfg, version, commit)

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

	databaseOptions := make([]postgres.Option, 0, 1)
	if cfg.AllowUserRegistrationOverride != nil {
		databaseOptions = append(databaseOptions, postgres.WithUserRegistrationOverride(*cfg.AllowUserRegistrationOverride))
	}
	database, err := postgres.Open(ctx, cfg.DatabaseURL, setupLogger, databaseOptions...)
	if err != nil {
		return setupFailure(setupLogger, "open database", "database_open_failed", err)
	}
	defer database.Close()

	webhooks := appwebhooks.NewWebhooks(database, secretCipher, logger.With("component", "webhooks"), cfg.PublicURL)
	access := appaccess.NewAccess(database)
	mutations := apppages.NewMutations(database, access, database, logger, webhooks)
	presence := apppages.NewPresence(database, access)
	discussions := apppages.NewDiscussions(database, access, nil, database, logger, webhooks)
	reviews := apppages.NewReviews(database, access, database, logger, webhooks)
	reviewDiscussions := apppages.NewReviewDiscussions(database, access, reviews, nil, database, logger, webhooks)
	bulk := apppages.NewBulk(database, mutations, database, logger, webhooks)

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

	serverLogger := logger.With("component", "server")
	home := apppages.NewHomeQuery(database, drafts, access)
	editor := apppages.NewEditor(pageLookup, groups, templates)
	editorSave := apppages.NewEditorSave(mutations, drafts, templates, serverLogger)
	viewPage := apppages.NewView(database, access, reviews, serverLogger)

	browserAuth, err := auth.ConfigureBrowserAuth(ctx, browserAuthConfig(cfg), database)
	if err != nil {
		return setupFailure(setupLogger, "configure browser auth", "browser_auth_failed", err)
	}
	bearerAuth := auth.NewBearer(database)

	renderer, err := pluginruntime.NewRenderer(
		ctx,
		database,
		secretCipher,
		authenticatedPluginRequest,
		logger,
		setupLogger,
		version,
		commit,
	)
	if err != nil {
		return err
	}
	defer closeRenderer(renderer, setupLogger)

	iconCatalog := renderer.IconCatalog()
	content := pagecontent.New(renderer)
	navigation.WithIconValidator(iconCatalog)
	mutations.WithIconValidator(iconCatalog).WithContentPreparer(content)
	discussions.WithContentPreparer(content)
	reviewDiscussions.WithContentPreparer(content)
	settings.WithIconValidator(iconCatalog)
	templates.WithIconValidator(iconCatalog)

	pluginUpdates := appplugins.NewPluginUpdates(
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
	views.WithRenderErrorHandler(httpresponse.InternalServerError)

	if cfg.DebugRenderTimings {
		renderer.EnableRenderTimings(logger.With("component", "markdown"))
		views.EnablePageTimings(logger.With("component", "handler"))
	}

	browserContext := endpoint.NewBrowserContext(viewer.New(
		preferences,
		navigation,
		database,
		settings,
		knowledge,
		notifications,
		access,
	), renderer)

	handler := httpserver.New(httpserver.Config{
		Assets:                appFS,
		Views:                 views,
		Renderer:              renderer,
		PluginUpdates:         pluginUpdates,
		BrowserAuth:           browserAuth,
		BearerAuth:            bearerAuth,
		Administration:        administration,
		Access:                access,
		PageLookup:            pageLookup,
		PageSearch:            pageSearch,
		PageDirectory:         pageDirectory,
		PageReports:           pageReports,
		PagePersonal:          pagePersonal,
		PageHistory:           pageHistory,
		PageRender:            pageRender,
		Drafts:                drafts,
		Groups:                groups,
		Knowledge:             knowledge,
		Notifications:         notifications,
		Media:                 media,
		Navigation:            navigation,
		PageMutations:         mutations,
		PagePresence:          presence,
		PageDiscussions:       discussions,
		PageReviews:           reviews,
		PageReviewDiscussions: reviewDiscussions,
		PageBulk:              bulk,
		Preferences:           preferences,
		RecycleBin:            recycleBin,
		Settings:              settings,
		System:                system,
		Templates:             templates,
		Tokens:                tokens,
		Users:                 users,
		Webhooks:              webhooks,
		Home:                  home,
		Editor:                editor,
		EditorSave:            editorSave,
		ViewPage:              viewPage,
		BrowserContext:        browserContext,
		Logger:                serverLogger,
		AccessLog:             cfg.AccessLog,
		ReadOnly:              cfg.ReadOnly,
	})

	if cfg.PluginUpdateCheckInterval > 0 {
		go pluginUpdates.Run(ctx)
	}

	if err := server.Run(
		ctx,
		cfg.ListenAddress,
		handler,
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

// runtimeInfo exposes deployment-managed values to administrator views without leaking flags into handlers.
func runtimeInfo(cfg flags.Config, secretCipher *secrets.Cipher) webview.RuntimeInfo {
	registrationOverrideConfigured := cfg.AllowUserRegistrationOverride != nil
	allowUserRegistrationOverride := false
	if registrationOverrideConfigured {
		allowUserRegistrationOverride = *cfg.AllowUserRegistrationOverride
	}

	return webview.RuntimeInfo{
		ListenAddress:                      cfg.ListenAddress,
		PublicURL:                          cfg.PublicURL,
		PDFURL:                             cfg.PDFURL,
		ReadOnly:                           cfg.ReadOnly,
		UserRegistrationOverrideConfigured: registrationOverrideConfigured,
		AllowUserRegistrationOverride:      allowUserRegistrationOverride,
		AuthModeOverride:                   string(cfg.AuthModeOverride),
		OIDCIssuerOverride:                 cfg.OIDCIssuer,
		OIDCClientIDOverride:               cfg.OIDCClientID,
		TrustedUsernameHeadersOverride:     cfg.TrustedUsernameHeaders,
		TrustedEmailHeadersOverride:        cfg.TrustedEmailHeaders,
		TrustedDisplayNameHeadersOverride:  cfg.TrustedDisplayNameHeaders,
		TrustedGroupHeadersOverride:        cfg.TrustedGroupHeaders,
		TrustedAdminGroupOverride:          cfg.TrustedAdminGroup,
		OIDCGroupClaimOverride:             cfg.OIDCGroupClaim,
		OIDCAdminGroupOverride:             cfg.OIDCAdminGroup,
		OIDCClientSecretConfigured:         cfg.OIDCClientSecret != "",
		OIDCSessionSecretConfigured:        len(cfg.OIDCSessionSecret) >= 32,
		EncryptionKeyConfigured:            secretCipher.Configured(),
		LocalLoginEnabled:                  cfg.LocalLogin,
		ThemeDirectory:                     cfg.ThemeDirectory,
		PluginUpdateCheckInterval:          pluginUpdateCheckIntervalLabel(cfg.PluginUpdateCheckInterval),
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

// pluginUpdateCheckIntervalLabel formats the deployment plugin update interval for administrator display.
func pluginUpdateCheckIntervalLabel(interval time.Duration) string {
	if interval <= 0 {
		return "Disabled"
	}

	return interval.String()
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
