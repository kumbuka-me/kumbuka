package server

import (
	"net/http"

	appkithttpserver "github.com/containeroo/appkit/httpserver"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
)

// newManagement constructs operational routes outside application middleware.
func newManagement(config Config) http.Handler {
	routes := appkithttpserver.ManagementRoutes{
		Health:    endpoint.Health(),
		Readiness: endpoint.Ready(config.System),
	}
	if config.MetricsEnabled {
		routes.Metrics = config.Metrics.Metrics()
	}
	return appkithttpserver.NewManagement(routes)
}
