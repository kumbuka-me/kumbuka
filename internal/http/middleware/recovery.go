package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// RecoverPanics converts handler panics into logged internal server errors.
func RecoverPanics(logger *slog.Logger) Middleware {
	return httpkitmiddleware.RecoverPanics(
		func(w http.ResponseWriter, _ *http.Request, recovered any) {
			httpresponse.InternalServerError(logger.With("operation", "request_panic"), w, fmt.Errorf("request panic: %v", recovered))
		},
		func(w http.ResponseWriter) bool {
			response, ok := w.(*httpresponse.RequestWriter)
			return ok && response.Status != 0
		},
	)
}
