package routes

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/handler"
	"github.com/kumbuka-me/kumbuka/internal/middleware"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// Config contains the fully constructed dependencies required by the HTTP router.
type Config struct {
	// Assets contains the embedded web application assets served by the router.
	Assets fs.FS
	// Views renders HTML responses and exposes the shared icon catalog.
	Views *handler.Views
	// Renderer renders Markdown and owns the active plugin manager.
	Renderer *markdown.Renderer
	// BrowserAuth contains browser authentication handlers and identity resolution.
	BrowserAuth auth.BrowserAuth
	// BearerAuth authenticates API requests that use personal access tokens.
	BearerAuth auth.Authenticator
	// Administration provides administrator-facing application use cases.
	Administration *service.Administration
	// Access provides page authorization and access-policy use cases.
	Access *service.Access
	// Catalog provides page lookup, search, and catalog use cases.
	Catalog *service.Catalog
	// Drafts provides page-draft use cases.
	Drafts *service.Drafts
	// Groups provides group-management use cases.
	Groups *service.Groups
	// Knowledge provides knowledge-graph and saved-search use cases.
	Knowledge *service.Knowledge
	// Notifications provides notification use cases.
	Notifications *service.Notifications
	// Media provides image and attachment use cases.
	Media *service.Media
	// Navigation provides navigation-tree and icon use cases.
	Navigation *service.Navigation
	// Pages provides page mutation and collaboration use cases.
	Pages *service.Pages
	// Preferences provides per-user preference use cases.
	Preferences *service.Preferences
	// RecycleBin provides deleted-page lifecycle use cases.
	RecycleBin *service.RecycleBin
	// Settings provides application-settings use cases.
	Settings *service.Settings
	// System provides health and setup-state use cases.
	System *service.System
	// Templates provides page-template use cases.
	Templates *service.Templates
	// Tokens provides personal and administrator token use cases.
	Tokens *service.Tokens
	// Users provides user and external-identity use cases.
	Users *service.Users
	// Webhooks provides webhook configuration and delivery use cases.
	Webhooks *service.Webhooks
	// ViewData loads shared page chrome and navigation data.
	ViewData *handler.ViewDataLoader
	// Logger records request, handler, and middleware diagnostics.
	Logger *slog.Logger
	// AccessLog enables request access logging when true.
	AccessLog bool
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
		adminAuthz:   middleware.RequireRole("admin"),
		editorAuthz:  middleware.RequireRole("admin", "editor"),
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
		middleware.SecurityHeaders(),
	)

	root := handler.HTMLProblems(mux, config.Views, "/auth/callback")
	return middleware.Chain(root, middlewares...)
}
