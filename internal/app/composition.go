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
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/postgres"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// httpConfig contains application and presentation dependencies used only while constructing HTTP endpoints.
type httpConfig struct {
	// Home loads dashboard page-list capabilities with application-owned access filtering.
	Home *apppages.HomeQuery
	// Editor loads create/edit page workflow data.
	Editor *apppages.Editor
	// EditorSave coordinates browser-editor page saves and draft cleanup.
	EditorSave *apppages.EditorSave
	// ViewPage loads authorized reading-page state.
	ViewPage *apppages.View
	// Assets contains the embedded web application assets served by HTTP endpoints.
	Assets fs.FS
	// Views renders HTML responses and exposes the shared icon catalog.
	Views *webview.Views
	// Renderer renders Markdown and owns the active plugin manager.
	Renderer *markdown.Renderer
	// PluginUpdates schedules, discovers, and downloads compatible first-party plugin releases.
	PluginUpdates *appplugins.PluginUpdates
	// BrowserAuth contains browser authentication handlers and identity resolution.
	BrowserAuth auth.BrowserAuth
	// BearerAuth authenticates API requests that use personal access tokens.
	BearerAuth auth.Authenticator
	// Administration provides administrator-facing application use cases.
	Administration *appadministration.Administration
	// Access provides page authorization and access-policy use cases.
	Access *appaccess.Access
	// PageLookup resolves direct page reads and aliases with actor authorization.
	PageLookup *apppages.Lookup
	// PageSearch provides actor-filtered page listing, search, and tag queries.
	PageSearch *apppages.Search
	// PageDirectory provides page aliases and inventory queries.
	PageDirectory *apppages.Directory
	// PageReports supplies authorized page data to reports and plugin capabilities.
	PageReports *apppages.Reports
	// PagePersonal owns actor-specific favorite and watch mutations.
	PagePersonal *apppages.Personal
	// PageHistory provides actor-authorized revision history.
	PageHistory *apppages.History
	// PageRender stores reusable page render artifacts.
	PageRender *apppages.RenderArtifacts
	// Drafts provides page-draft use cases.
	Drafts *apppages.Drafts
	// Groups provides group-management use cases.
	Groups *appgroups.Groups
	// Knowledge provides knowledge-graph and saved-search use cases.
	Knowledge *appsearch.Knowledge
	// Notifications provides notification use cases.
	Notifications *appnotifications.Notifications
	// Media provides image and attachment use cases.
	Media *appmedia.Media
	// Navigation provides navigation-tree and icon use cases.
	Navigation *appnavigation.Navigation
	// Pages provides page mutation and collaboration use cases.
	Pages *apppages.Pages
	// Preferences provides per-user preference use cases.
	Preferences *apppreferences.Preferences
	// RecycleBin provides deleted-page lifecycle use cases.
	RecycleBin *apprecyclebin.RecycleBin
	// Settings provides application-settings use cases.
	Settings *appsettings.Settings
	// System provides health and setup-state use cases.
	System *appsystem.System
	// Templates provides page-template use cases.
	Templates *apptemplates.Templates
	// Tokens provides personal and administrator token use cases.
	Tokens *apptokens.Tokens
	// Users provides user and external-identity use cases.
	Users *appusers.Users
	// Webhooks provides webhook configuration and delivery use cases.
	Webhooks *appwebhooks.Webhooks
	// BrowserContext loads shared browser context and presentation contributions.
	BrowserContext *endpoint.BrowserContext
	// Logger records request and endpoint diagnostics.
	Logger *slog.Logger
	// AccessLog enables request access logging when true.
	AccessLog bool
	// ReadOnly blocks state-changing application routes while preserving authentication flows.
	ReadOnly bool
}

// newRouteConfig constructs application use cases while leaving runtime-bound HTTP dependencies unset.
func newRouteConfig(
	appFS fs.FS,
	cfg flags.Config,
	database *postgres.Store,
	secretCipher *secrets.Cipher,
	logger *slog.Logger,
) httpConfig {
	webhooks := appwebhooks.NewWebhooks(
		database,
		secretCipher,
		logger.With("component", "webhooks"),
		cfg.PublicURL,
	)

	access := appaccess.NewAccess(database)
	pages := apppages.NewPages(database, access, logger, webhooks)

	config := httpConfig{
		Assets:         appFS,
		Administration: appadministration.NewAdministration(database),
		Access:         access,
		PageLookup:     apppages.NewLookup(database, access),
		PageSearch:     apppages.NewSearch(database, access),
		PageDirectory:  apppages.NewDirectory(database, access),
		PageReports:    apppages.NewReports(database, access),
		PagePersonal:   apppages.NewPersonal(database, access),
		PageHistory:    apppages.NewHistory(database, access),
		PageRender:     apppages.NewRenderArtifacts(database),
		Drafts:         apppages.NewDrafts(database),
		Groups:         appgroups.NewGroups(database),
		Knowledge:      appsearch.NewKnowledge(database, access),
		Notifications:  appnotifications.NewNotifications(database),
		Media:          appmedia.NewMedia(database),
		Navigation:     appnavigation.NewNavigation(database, access),
		Pages:          pages,
		Preferences:    apppreferences.NewPreferences(database),
		RecycleBin:     apprecyclebin.NewRecycleBin(database),
		Settings:       appsettings.NewSettings(database, secretCipher).WithLogger(logger.With("component", "settings")),
		System:         appsystem.NewSystem(database).WithLogger(logger.With("component", "system")),
		Templates:      apptemplates.NewTemplates(database),
		Tokens:         apptokens.NewTokens(database),
		Users:          appusers.NewUsers(database).WithLogger(logger.With("component", "users")),
		Webhooks:       webhooks,
		Logger:         logger.With("component", "server"),
		AccessLog:      cfg.AccessLog,
		ReadOnly:       cfg.ReadOnly,
	}
	config.Home = apppages.NewHomeQuery(database, config.Drafts, config.Access)
	config.Editor = apppages.NewEditor(config.PageLookup, config.Groups, config.Templates)
	config.EditorSave = apppages.NewEditorSave(config.Pages, config.Drafts, config.Templates, config.Logger)
	config.ViewPage = apppages.NewView(database, config.Access, config.Pages, config.Logger)
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
func configurePluginAwareServices(config *httpConfig, renderer *markdown.Renderer, catalog *icons.Catalog) {
	config.Navigation.WithIconCatalog(catalog)
	config.Pages.WithIconCatalog(catalog).WithRenderer(renderer)
	config.Settings.WithIconCatalog(catalog)
	config.Templates.WithIconCatalog(catalog)
}

// newBrowserContext wires the shared authenticated browser-context aggregation boundary.
func newBrowserContext(config httpConfig, database *postgres.Store) *endpoint.BrowserContext {
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
