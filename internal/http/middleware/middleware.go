package middleware

import (
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
)

// Middleware wraps an HTTP handler with cross-cutting behavior.
type Middleware = httpkitmiddleware.Middleware

// Chain wraps a handler with middleware in declaration order.
func Chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	return httpkitmiddleware.Chain(handler, middlewares...)
}
