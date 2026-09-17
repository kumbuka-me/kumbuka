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
	"github.com/kumbuka-me/kumbuka/internal/handler"
	"github.com/kumbuka-me/kumbuka/internal/routes"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/logging"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/pkg/store"
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

	availableThemes, err := themes.Load(cfg.ThemeDirectory)
	if err != nil {
		setupLogger.Error(
			"load themes",
			"event", "theme_load_failed",
			"error", err,
		)
		return err
	}

	secretCipher, err := secrets.New(cfg.EncryptionKey)
	if err != nil {
		setupLogger.Error(
			"configure application encryption",
			"event", "application_encryption_failed",
			"error", err,
		)
		return err
	}

	storeOptions := make([]store.Option, 0, 1)
	if cfg.AllowUserRegistrationOverride != nil {
		storeOptions = append(
			storeOptions,
			store.WithUserRegistrationOverride(*cfg.AllowUserRegistrationOverride),
		)
	}

	database, err := store.Open(ctx, cfg.DatabaseURL, setupLogger, storeOptions...)
	if err != nil {
		setupLogger.Error(
			"open database",
			"event", "database_open_failed",
			"error", err,
		)
		return err
	}
	defer database.Close()

	// Construct application services here so internal/app remains the single
	// composition root. The routing layer only receives ready-to-use
	// dependencies and decides which handlers consume them.
	administrationUseCases := service.NewAdministration(database)
	accessUseCases := service.NewAccess(database)
	catalogUseCases := service.NewCatalog(database)
	draftUseCases := service.NewDrafts(database)
	groupUseCases := service.NewGroups(database)
	knowledgeUseCases := service.NewKnowledge(database)
	notificationUseCases := service.NewNotifications(database)
	mediaUseCases := service.NewMedia(database)
	navigationUseCases := service.NewNavigation(database)
	webhookUseCases := service.NewWebhooks(
		database,
		secretCipher,
		logger.With("component", "webhooks"),
		cfg.PublicURL,
	)
	pageUseCases := service.NewPages(database, logger, webhookUseCases)
	preferenceUseCases := service.NewPreferences(database)
	recycleBinUseCases := service.NewRecycleBin(database)
	settingsUseCases := service.NewSettings(database, secretCipher)
	systemUseCases := service.NewSystem(database)
	templateUseCases := service.NewTemplates(database)
	tokenUseCases := service.NewTokens(database)
	userUseCases := service.NewUsers(database)

	browserAuth, err := auth.ConfigureBrowserAuth(
		ctx,
		auth.BrowserConfig{
			ModeOverride: cfg.AuthModeOverride,
			TrustedProxy: auth.TrustedProxyHeaders{
				Username:    cfg.TrustedUsernameHeaders,
				Email:       cfg.TrustedEmailHeaders,
				DisplayName: cfg.TrustedDisplayNameHeaders,
			},
			OIDC: auth.OIDCConfig{
				ClientID:      cfg.OIDCClientID,
				ClientSecret:  cfg.OIDCClientSecret,
				Issuer:        cfg.OIDCIssuer,
				SessionSecret: cfg.OIDCSessionSecret,
				PublicURL:     cfg.PublicURL,
			},
			LocalLoginEnabled: cfg.LocalLogin,
		},
		database,
	)
	if err != nil {
		setupLogger.Error(
			"configure browser auth",
			"event", "browser_auth_failed",
			"error", err,
		)
		return err
	}

	bearerAuth := auth.NewBearer(database)

	ctx, stop := server.SignalContext(ctx)
	defer stop()

	pluginArchives, err := plugins.Archives()
	if err != nil {
		setupLogger.Error(
			"load bundled plugins",
			"event", "plugin_packages_load_failed",
			"error", err,
		)
		return err
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
		setupLogger.Error(
			"create markdown renderer",
			"event", "markdown_renderer_failed",
			"error", err,
		)
		return err
	}

	renderer.SetArtifactBuild(version, commit)
	defer func() { _ = renderer.Close(context.Background()) }()

	iconCatalog := renderer.IconCatalog()

	navigationUseCases.WithIconCatalog(iconCatalog)
	pageUseCases.WithIconCatalog(iconCatalog).WithRenderer(renderer)
	settingsUseCases.WithIconCatalog(iconCatalog)
	templateUseCases.WithIconCatalog(iconCatalog)

	registrationOverrideConfigured := cfg.AllowUserRegistrationOverride != nil
	allowUserRegistrationOverride := false
	if registrationOverrideConfigured {
		allowUserRegistrationOverride = *cfg.AllowUserRegistrationOverride
	}

	views, err := handler.NewViews(
		appFS,
		logger,
		version,
		commit,
		availableThemes,
		handler.RuntimeInfo{
			ListenAddress:                      cfg.ListenAddress,
			PublicURL:                          cfg.PublicURL,
			PDFURL:                             cfg.PDFURL,
			UserRegistrationOverrideConfigured: registrationOverrideConfigured,
			AllowUserRegistrationOverride:      allowUserRegistrationOverride,
			AuthModeOverride:                   string(cfg.AuthModeOverride),
			OIDCIssuerOverride:                 cfg.OIDCIssuer,
			OIDCClientIDOverride:               cfg.OIDCClientID,
			TrustedUsernameHeadersOverride:     cfg.TrustedUsernameHeaders,
			TrustedEmailHeadersOverride:        cfg.TrustedEmailHeaders,
			TrustedDisplayNameHeadersOverride:  cfg.TrustedDisplayNameHeaders,
			OIDCClientSecretConfigured:         cfg.OIDCClientSecret != "",
			OIDCSessionSecretConfigured:        len(cfg.OIDCSessionSecret) >= 32,
			EncryptionKeyConfigured:            secretCipher.Configured(),
			LocalLoginEnabled:                  cfg.LocalLogin,
			ThemeDirectory:                     cfg.ThemeDirectory,
		},
		iconCatalog,
	)
	if err != nil {
		setupLogger.Error(
			"create views",
			"event", "views_create_failed",
			"error", err,
		)
		return err
	}

	if cfg.DebugRenderTimings {
		renderer.EnableRenderTimings(logger.With("component", "markdown"))
		views.EnablePageTimings(logger.With("component", "handler"))
	}

	viewDataUseCases := handler.NewViewDataLoader(
		preferenceUseCases,
		navigationUseCases,
		catalogUseCases,
		settingsUseCases,
		knowledgeUseCases,
		notificationUseCases,
		accessUseCases,
		renderer,
	)

	router := routes.New(routes.Config{
		Assets:         appFS,
		Views:          views,
		Renderer:       renderer,
		BrowserAuth:    browserAuth,
		BearerAuth:     bearerAuth,
		Administration: administrationUseCases,
		Access:         accessUseCases,
		Catalog:        catalogUseCases,
		Drafts:         draftUseCases,
		Groups:         groupUseCases,
		Knowledge:      knowledgeUseCases,
		Notifications:  notificationUseCases,
		Media:          mediaUseCases,
		Navigation:     navigationUseCases,
		Pages:          pageUseCases,
		Preferences:    preferenceUseCases,
		RecycleBin:     recycleBinUseCases,
		Settings:       settingsUseCases,
		System:         systemUseCases,
		Templates:      templateUseCases,
		Tokens:         tokenUseCases,
		Users:          userUseCases,
		Webhooks:       webhookUseCases,
		ViewData:       viewDataUseCases,
		Logger:         logger.With("component", "server"),
		AccessLog:      cfg.AccessLog,
	})

	if err := server.Run(
		ctx,
		cfg.ListenAddress,
		router,
		setupLogger,
		server.WithMaxHeaderValueCount(100),
	); err != nil {
		setupLogger.Error(
			"run server",
			"event", "server_run_failed",
			"error", err,
		)
		return err
	}

	return nil
}
