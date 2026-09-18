package app

import (
	"io/fs"
	"log/slog"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/pluginupdate"
	"github.com/kumbuka-me/kumbuka/internal/routes"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/store"
)

// newRouteConfig constructs application use cases while leaving runtime-bound HTTP dependencies unset.
func newRouteConfig(
	appFS fs.FS,
	cfg flags.Config,
	database *store.Store,
	secretCipher *secrets.Cipher,
	logger *slog.Logger,
) routes.Config {
	webhooks := service.NewWebhooks(
		database,
		secretCipher,
		logger.With("component", "webhooks"),
		cfg.PublicURL,
	)

	return routes.Config{
		Assets:         appFS,
		PluginUpdates:  pluginupdate.New(pluginupdate.DefaultCatalogURL),
		Administration: service.NewAdministration(database),
		Access:         service.NewAccess(database),
		Catalog:        service.NewCatalog(database),
		Drafts:         service.NewDrafts(database),
		Groups:         service.NewGroups(database),
		Knowledge:      service.NewKnowledge(database),
		Notifications:  service.NewNotifications(database),
		Media:          service.NewMedia(database),
		Navigation:     service.NewNavigation(database),
		Pages:          service.NewPages(database, logger, webhooks),
		Preferences:    service.NewPreferences(database),
		RecycleBin:     service.NewRecycleBin(database),
		Settings:       service.NewSettings(database, secretCipher).WithLogger(logger.With("component", "settings")),
		System:         service.NewSystem(database).WithLogger(logger.With("component", "system")),
		Templates:      service.NewTemplates(database),
		Tokens:         service.NewTokens(database),
		Users:          service.NewUsers(database).WithLogger(logger.With("component", "users")),
		Webhooks:       webhooks,
		Logger:         logger.With("component", "server"),
		AccessLog:      cfg.AccessLog,
		ReadOnly:       cfg.ReadOnly,
	}
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
	}
}

// configurePluginAwareServices installs runtime catalogs and rendering into services that validate plugin-owned data.
func configurePluginAwareServices(config *routes.Config, renderer *markdown.Renderer, catalog *icons.Catalog) {
	config.Navigation.WithIconCatalog(catalog)
	config.Pages.WithIconCatalog(catalog).WithRenderer(renderer)
	config.Settings.WithIconCatalog(catalog)
	config.Templates.WithIconCatalog(catalog)
}

// newViewDataLoader wires the shared authenticated view-data aggregation boundary.
func newViewDataLoader(config routes.Config) *webview.Loader {
	return webview.NewLoader(
		config.Preferences,
		config.Navigation,
		config.Catalog,
		config.Settings,
		config.Knowledge,
		config.Notifications,
		config.Access,
		config.Renderer,
	)
}
