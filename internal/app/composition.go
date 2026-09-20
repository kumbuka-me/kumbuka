package app

import (
	"io/fs"
	"log/slog"
	"time"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	appadministration "github.com/kumbuka-me/kumbuka/internal/application/administration"
	appgroups "github.com/kumbuka-me/kumbuka/internal/application/groups"
	appmedia "github.com/kumbuka-me/kumbuka/internal/application/media"
	appnavigation "github.com/kumbuka-me/kumbuka/internal/application/navigation"
	appnotifications "github.com/kumbuka-me/kumbuka/internal/application/notifications"
	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
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
	httpserver "github.com/kumbuka-me/kumbuka/internal/http/server"
	"github.com/kumbuka-me/kumbuka/internal/pagecontent"
	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// newRouteConfig constructs application use cases while leaving runtime-bound HTTP dependencies unset.
func newRouteConfig(
	appFS fs.FS,
	cfg flags.Config,
	database *postgres.Store,
	secretCipher *secrets.Cipher,
	logger *slog.Logger,
) httpserver.Config {
	webhooks := appwebhooks.NewWebhooks(
		database,
		secretCipher,
		logger.With("component", "webhooks"),
		cfg.PublicURL,
	)

	access := appaccess.NewAccess(database)
	mutations := apppages.NewMutations(database, access, database, logger, webhooks)
	presence := apppages.NewPresence(database, access)
	discussions := apppages.NewDiscussions(database, access, nil, database, logger, webhooks)
	reviews := apppages.NewReviews(database, access, database, logger, webhooks)
	reviewDiscussions := apppages.NewReviewDiscussions(database, access, reviews, nil, database, logger, webhooks)
	bulk := apppages.NewBulk(database, mutations, database, logger, webhooks)

	config := httpserver.Config{
		Assets:                appFS,
		Administration:        appadministration.NewAdministration(database),
		Access:                access,
		PageLookup:            apppages.NewLookup(database, access),
		PageSearch:            apppages.NewSearch(database, access),
		PageDirectory:         apppages.NewDirectory(database, access),
		PageReports:           apppages.NewReports(database, access),
		PagePersonal:          apppages.NewPersonal(database, access),
		PageHistory:           apppages.NewHistory(database, access),
		PageRender:            apppages.NewRenderArtifacts(database),
		Drafts:                apppages.NewDrafts(database),
		Groups:                appgroups.NewGroups(database),
		Knowledge:             appsearch.NewKnowledge(database, access),
		Notifications:         appnotifications.NewNotifications(database),
		Media:                 appmedia.NewMedia(database),
		Navigation:            appnavigation.NewNavigation(database, access),
		PageMutations:         mutations,
		PagePresence:          presence,
		PageDiscussions:       discussions,
		PageReviews:           reviews,
		PageReviewDiscussions: reviewDiscussions,
		PageBulk:              bulk,
		Preferences:           apppreferences.NewPreferences(database),
		RecycleBin:            apprecyclebin.NewRecycleBin(database),
		Settings:              appsettings.NewSettings(database, secretCipher).WithLogger(logger.With("component", "settings")),
		System:                appsystem.NewSystem(database).WithLogger(logger.With("component", "system")),
		Templates:             apptemplates.NewTemplates(database),
		Tokens:                apptokens.NewTokens(database),
		Users:                 appusers.NewUsers(database, credential.Passwords{}).WithLogger(logger.With("component", "users")),
		Webhooks:              webhooks,
		Logger:                logger.With("component", "server"),
		AccessLog:             cfg.AccessLog,
		ReadOnly:              cfg.ReadOnly,
	}
	config.Home = apppages.NewHomeQuery(database, config.Drafts, config.Access)
	config.Editor = apppages.NewEditor(config.PageLookup, config.Groups, config.Templates)
	config.EditorSave = apppages.NewEditorSave(config.PageMutations, config.Drafts, config.Templates, config.Logger)
	config.ViewPage = apppages.NewView(database, config.Access, config.PageReviews, config.Logger)
	return config
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

// pluginUpdateCheckIntervalLabel formats the deployment plugin update interval for administrator display.
func pluginUpdateCheckIntervalLabel(interval time.Duration) string {
	if interval <= 0 {
		return "Disabled"
	}

	return interval.String()
}

// configurePluginAwareServices installs runtime catalogs and rendering into services that validate plugin-owned data.
func configurePluginAwareServices(config *httpserver.Config, renderer *markdown.Renderer, catalog *icons.Catalog) {
	content := pagecontent.New(renderer)
	config.Navigation.WithIconValidator(catalog)
	config.PageMutations.WithIconValidator(catalog).WithContentPreparer(content)
	config.PageDiscussions.WithContentPreparer(content)
	config.PageReviewDiscussions.WithContentPreparer(content)
	config.Settings.WithIconValidator(catalog)
	config.Templates.WithIconValidator(catalog)
}

// newBrowserContext wires the shared authenticated browser-context aggregation boundary.
func newBrowserContext(config httpserver.Config, database *postgres.Store) *endpoint.BrowserContext {
	return endpoint.NewBrowserContext(viewer.New(
		config.Preferences,
		config.Navigation,
		database,
		config.Settings,
		config.Knowledge,
		config.Notifications,
		config.Access,
	), config.Renderer,
	)
}
