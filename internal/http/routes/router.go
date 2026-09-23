package routes

import (
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Config contains transport-level dependencies required to register and wrap HTTP routes.
type Config struct {
	// Views renders themed browser problem responses around selected public authentication routes.
	Views *webview.Views
	// BrowserAuth resolves browser identities used by browser and mixed authentication policies.
	BrowserAuth auth.BrowserAuth
	// BearerAuth resolves personal-access-token identities used by API and media policies.
	BearerAuth auth.Authenticator
	// Logger records request and middleware diagnostics.
	Logger *slog.Logger
	// AccessLog enables request access logging when true.
	AccessLog bool
	// ReadOnly blocks state-changing application routes while preserving authentication flows.
	ReadOnly bool
}

// routePolicies groups authentication and authorization middleware used while registering routes.
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

// Router registers preconstructed HTTP endpoints and applies transport policies.
type Router struct {
	// mux owns the HTTP method/path registrations.
	mux *http.ServeMux
	// config contains transport-level runtime settings.
	config Config
	// policies contains reusable authentication and role middleware.
	policies routePolicies
}

// New constructs an empty application router with its authentication and authorization policies.
func New(config Config) *Router {
	return &Router{
		mux:    http.NewServeMux(),
		config: config,
		policies: routePolicies{
			browserAuthn: middleware.Authenticate(config.Logger, config.BrowserAuth.Authenticator),
			mediaAuthn:   middleware.Authenticate(config.Logger, config.BearerAuth, config.BrowserAuth.Authenticator),
			apiAuthn:     middleware.AuthenticateAPI(config.Logger, config.BearerAuth, config.BrowserAuth.Authenticator),
			adminAuthz:   middleware.RequireRole(domain.UserRoleAdmin),
			editorAuthz:  middleware.RequireRole(domain.UserRoleAdmin, domain.UserRoleEditor),
		},
	}
}

// Handle registers one preconstructed handler for an HTTP method/path pattern.
func (r *Router) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
}

// HandleFunc registers one preconstructed handler function for an HTTP method/path pattern.
func (r *Router) HandleFunc(pattern string, handler http.HandlerFunc) {
	r.mux.HandleFunc(pattern, handler)
}

// Browser applies browser-session authentication to a handler.
func (r *Router) Browser(handler http.Handler) http.Handler {
	return r.policies.browserAuthn(handler)
}

// Media applies mixed browser or bearer authentication to a media handler.
func (r *Router) Media(handler http.Handler) http.Handler {
	return r.policies.mediaAuthn(handler)
}

// API applies API authentication to a handler.
func (r *Router) API(handler http.Handler) http.Handler {
	return r.policies.apiAuthn(handler)
}

// Admin restricts a handler to administrators after authentication.
func (r *Router) Admin(handler http.Handler) http.Handler {
	return r.policies.adminAuthz(handler)
}

// Editor restricts a handler to editors and administrators after authentication.
func (r *Router) Editor(handler http.Handler) http.Handler {
	return r.policies.editorAuthz(handler)
}

// Handler finalizes the router with global request middleware.
func (r *Router) Handler() http.Handler {
	middlewares := []middleware.Middleware{
		middleware.SecurityHeaders(),
		middleware.RequestContext(),
	}
	if r.config.AccessLog {
		middlewares = append(middlewares, middleware.AccessLog(r.config.Logger))
	}
	middlewares = append(
		middlewares,
		middleware.RecoverPanics(r.config.Logger),
		middleware.RejectCrossSiteWrites(r.config.Logger),
	)
	if r.config.ReadOnly {
		middlewares = append(middlewares, middleware.ReadOnly())
	}

	root := middleware.Chain(r.mux, middlewares...)
	return endpoint.HTMLProblems(
		root,
		r.config.Views,
		"/auth/login",
		"/auth/local",
		"/auth/callback",
		"/setup",
	)
}
