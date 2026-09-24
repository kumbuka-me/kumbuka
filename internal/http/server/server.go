package server

import (
	"net/http"

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
	return router.Handler()
}
