package server

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
)

// newManagement constructs operational routes outside application middleware.
func newManagement(config Config) http.Handler {
	mux := http.NewServeMux()
	registerOperationalRoutes(mux, config)
	return mux
}

// registerOperationalRoutes registers operational routes outside application middleware.
func registerOperationalRoutes(mux *http.ServeMux, config Config) {
	mux.Handle("GET /healthz", endpoint.Health())
	mux.Handle("/healthz", methodNotAllowed(http.MethodGet, http.MethodHead))
	mux.Handle("GET /readyz", endpoint.Ready(config.System))
	mux.Handle("/readyz", methodNotAllowed(http.MethodGet, http.MethodHead))
	if config.MetricsEnabled {
		mux.Handle("GET /metrics", config.Metrics.Metrics())
		mux.Handle("/metrics", methodNotAllowed(http.MethodGet, http.MethodHead))
	}
}
