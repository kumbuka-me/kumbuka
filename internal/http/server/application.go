package server

import (
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
)

// newApplication constructs browser and API routes with application middleware.
func newApplication(config Config) http.Handler {
	applicationMux := http.NewServeMux()

	registerPublicRoutes(applicationMux, config)
	registerBrowserRoutes(applicationMux, config)
	registerAdminRoutes(applicationMux, config)
	registerPageRoutes(applicationMux, config)
	registerAPIRoutes(applicationMux, config)
	registerFallbackRoutes(applicationMux, config)

	middlewares := []httpkitmiddleware.Middleware{
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
	if config.PerformanceDiagnostics {
		middlewares = append(middlewares, middleware.PerformanceDiagnostics(config.Logger))
	}
	if config.ReadOnly {
		middlewares = append(middlewares, middleware.ReadOnly())
	}

	application := httpkitmiddleware.Chain(applicationMux, middlewares...)
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

	return application
}
