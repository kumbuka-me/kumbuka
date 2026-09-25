package server

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// registerFallbackRoutes registers API and authenticated browser not-found handlers.
func registerFallbackRoutes(mux *http.ServeMux, config Config) {
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	})
	browserAuthn := middleware.Authenticate(config.Logger, config.BrowserAuth.Authenticator)
	mux.Handle("GET /", browserAuthn(endpoint.NotFound(config.BrowserContext, config.Views)))
}
