package routes

import (
	"io/fs"
	"log/slog"
	"net/http"

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
	appwebhooks "github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/handler"
	"github.com/kumbuka-me/kumbuka/internal/middleware"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// Config contains the fully constructed dependencies required by the HTTP router.
type Config struct {
	// Assets contains the embedded web application assets served by the router.
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
	// Catalog provides page lookup, search, and catalog use cases.
	Catalog *apppages.Catalog
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
	// ViewData loads shared page chrome and navigation data.
	ViewData *webview.Loader
	// Logger records request, handler, and middleware diagnostics.
	Logger *slog.Logger
	// AccessLog enables request access logging when true.
	AccessLog bool
	// ReadOnly blocks state-changing application routes while preserving authentication flows.
	ReadOnly bool
}

// routePolicies groups authentication and authorization middleware used during route registration.
type routePolicies struct {
	// browserAuthn authenticates browser-only routes.
	browserAuthn middleware.Middleware
	// mediaAuthn accepts either browser sessions or bearer tokens for media routes.
	mediaAuthn middleware.Middleware
	// apiAuthn accepts either browser sessions or bearer tokens for API routes.
	apiAuthn middleware.Middleware
	// adminAuthz restricts a route to administrators.
	adminAuthz middleware.Middleware
	// editorAuthz restricts a route to editors and administrators.
	editorAuthz middleware.Middleware
}

// New constructs the application router and its authentication and authorization policies.
func New(config Config) http.Handler {
	mux := http.NewServeMux()

	policies := routePolicies{
		browserAuthn: middleware.Authenticate(config.Logger, config.BrowserAuth.Authenticator),
		mediaAuthn:   middleware.Authenticate(config.Logger, config.BearerAuth, config.BrowserAuth.Authenticator),
		apiAuthn:     middleware.AuthenticateAPI(config.Logger, config.BearerAuth, config.BrowserAuth.Authenticator),
		adminAuthz:   middleware.RequireRole(domain.UserRoleAdmin),
		editorAuthz:  middleware.RequireRole(domain.UserRoleAdmin, domain.UserRoleEditor),
	}

	addRoutes(mux, config, policies)

	middlewares := []middleware.Middleware{middleware.RequestContext()}
	if config.AccessLog {
		middlewares = append(middlewares, middleware.AccessLog(config.Logger))
	}
	middlewares = append(
		middlewares,
		middleware.RecoverPanics(config.Logger),
		middleware.RejectCrossSiteWrites(config.Logger),
	)
	if config.ReadOnly {
		middlewares = append(middlewares, middleware.ReadOnly())
	}
	middlewares = append(middlewares, middleware.SecurityHeaders())

	root := handler.HTMLProblems(
		mux,
		config.Views,
		"/auth/login",
		"/auth/local",
		"/auth/callback",
		"/setup",
	)
	return middleware.Chain(root, middlewares...)
}
