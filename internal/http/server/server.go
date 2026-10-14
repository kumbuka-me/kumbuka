package server

import (
	"net/http"
	"strings"

	appkithttpserver "github.com/containeroo/appkit/httpserver"
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

// New constructs Kumbuka routes and mounts only the public application under its deployment prefix.
func New(config Config) appkithttpserver.Handlers {
	return appkithttpserver.New(appkithttpserver.Config{
		Application: newApplication(config),
		Management:  newManagement(config),
		RoutePrefix: config.RoutePrefix,
	})
}

// methodNotAllowed rejects unsupported methods on operational endpoints.
func methodNotAllowed(methods ...string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Allow", strings.Join(methods, ", "))
		http.Error(response, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
	})
}
