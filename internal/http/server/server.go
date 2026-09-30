package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	httpgrace "github.com/containeroo/httpgrace/server"
	"golang.org/x/sync/errgroup"
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
	Application http.Handler
	Management  http.Handler
}

// New constructs both HTTP applications with the same deployment prefix.
func New(config Config) Handlers {
	return Handlers{
		Application: newApplication(config),
		Management:  newManagement(config),
	}
}

// Run serves both handlers and stops the remaining listener when either exits.
func Run(ctx context.Context, applicationAddress, managementAddress string, handlers Handlers, logger *slog.Logger) error {
	listenerContext, cancel := context.WithCancel(ctx)
	defer cancel()

	group, groupContext := errgroup.WithContext(listenerContext)
	group.Go(func() error {
		defer cancel()
		return httpgrace.Run(groupContext, applicationAddress, handlers.Application, logger, httpgrace.WithMaxHeaderValueCount(100))
	})
	group.Go(func() error {
		defer cancel()
		return httpgrace.Run(groupContext, managementAddress, handlers.Management, logger, httpgrace.WithMaxHeaderValueCount(100))
	})
	return group.Wait()
}

// methodNotAllowed rejects unsupported methods on operational endpoints.
func methodNotAllowed(methods ...string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Allow", strings.Join(methods, ", "))
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})
}
