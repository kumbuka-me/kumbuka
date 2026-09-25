package server

import (
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/routes"
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
	router := routes.New(routes.Config{
		Views:       config.Views,
		BrowserAuth: config.BrowserAuth,
		BearerAuth:  config.BearerAuth,
		Logger:      config.Logger,
		AccessLog:   config.AccessLog,
		ReadOnly:    config.ReadOnly,
	})
	addRoutes(router, config)

	return operationalRoutes(router.Handler(), config)
}

// operationalRoutes keeps scrape and health traffic outside application request instrumentation.
func operationalRoutes(application http.Handler, config Config) http.Handler {
	root := http.NewServeMux()
	root.Handle("GET /healthz", endpoint.Health(config.System))
	root.Handle("/healthz", methodNotAllowed(http.MethodGet, http.MethodHead))

	if config.MetricsEnabled {
		root.Handle("GET /metrics", config.Metrics.Metrics())
		root.Handle("/metrics", methodNotAllowed(http.MethodGet, http.MethodHead))
		application = config.Metrics.InstrumentHTTP(application)
	}

	// ServeMux records the outer fallback pattern before invoking application.
	// Clear it so the inner application mux can supply the stable route label,
	// while middleware rejections that happen before inner routing stay unmatched.
	root.Handle("/", http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		request.Pattern = ""
		application.ServeHTTP(response, request)
	}))

	return root
}

// methodNotAllowed rejects unsupported methods on operational endpoints without falling through to the application.
func methodNotAllowed(methods ...string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Allow", strings.Join(methods, ", "))
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})
}
