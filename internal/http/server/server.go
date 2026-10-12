package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	grace "github.com/containeroo/httpkit/grace"
	httpkitserver "github.com/containeroo/httpkit/server"
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

// Handlers contains the application and operational management handlers.
type Handlers struct {
	// Application serves public application traffic.
	Application http.Handler
	// Management serves operational health and metrics traffic.
	Management http.Handler
}

// New constructs both HTTP applications with the same deployment prefix.
func New(config Config) Handlers {
	return Handlers{
		Application: newApplication(config),
		Management:  newManagement(config),
	}
}

// Run serves both handlers and stops the remaining listener when either exits.
func Run(
	ctx context.Context,
	appAddress string,
	managementAddress string,
	handlers Handlers,
	logger *slog.Logger,
) error {
	return httpkitserver.Run(ctx,
		httpkitserver.Listener{
			Address: appAddress,
			Handler: handlers.Application,
			Logger:  logger.With("server", "application"),
			Options: []grace.Option{grace.WithMaxHeaderValueCount(100)},
		},
		httpkitserver.Listener{
			Address: managementAddress,
			Handler: handlers.Management,
			Logger:  logger.With("server", "management"),
			Options: []grace.Option{grace.WithMaxHeaderValueCount(100)},
		},
	)
}

// methodNotAllowed rejects unsupported methods on operational endpoints.
func methodNotAllowed(methods ...string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Allow", strings.Join(methods, ", "))
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})
}
