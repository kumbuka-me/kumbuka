package server

import (
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
)

// Config groups the capabilities used to construct HTTP routes.
type Config struct {
	// InfrastructureConfig supplies transport, presentation, and runtime options.
	InfrastructureConfig
	// AuthenticationConfig supplies browser and API authenticators.
	AuthenticationConfig
	// BrowserConfig supplies shared browser queries and preferences.
	BrowserConfig
	// AdministrationConfig supplies management operations.
	AdministrationConfig
	// PageQueryConfig supplies authorized page reads.
	PageQueryConfig
	// PageWorkflowConfig supplies page mutations and editor workflows.
	PageWorkflowConfig
}

// New constructs the fully routed HTTP application.
func New(config Config) http.Handler {
	applicationMux := http.NewServeMux()
	registerPublicRoutes(applicationMux, config)
	registerBrowserRoutes(applicationMux, config)
	registerAdminRoutes(applicationMux, config)
	registerPageRoutes(applicationMux, config)
	registerAPIRoutes(applicationMux, config)
	registerFallbackRoutes(applicationMux, config)

	middlewares := []middleware.Middleware{
		middleware.SecurityHeaders(),
		middleware.RequestContext(),
	}
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

	application := middleware.Chain(applicationMux, middlewares...)
	application = endpoint.HTMLProblems(
		application,
		config.Views,
		"/auth/login",
		"/auth/local",
		"/auth/callback",
		"/setup",
	)
	if config.MetricsEnabled {
		application = config.Metrics.InstrumentHTTP(application)
	}

	root := http.NewServeMux()
	root.Handle("GET /healthz", endpoint.Health(config.System))
	root.Handle("/healthz", methodNotAllowed(http.MethodGet, http.MethodHead))
	if config.MetricsEnabled {
		root.Handle("GET /metrics", config.Metrics.Metrics())
		root.Handle("/metrics", methodNotAllowed(http.MethodGet, http.MethodHead))
	}

	// The outer fallback matches every application request before the inner mux.
	// Clear that match so HTTP metrics can record the inner stable route pattern.
	root.Handle("/", http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		request.Pattern = ""
		application.ServeHTTP(response, request)
	}))

	return root
}

// methodNotAllowed rejects unsupported methods on operational endpoints without falling through to application routes.
func methodNotAllowed(methods ...string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Allow", strings.Join(methods, ", "))
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})
}
