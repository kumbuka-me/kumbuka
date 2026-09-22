package middleware

import (
	"log/slog"
	"net/http"
	"time"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// AccessLog records request method, path, and elapsed time after handling.
func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()

			next.ServeHTTP(w, r)
			requestLogger := logger
			if request, ok := w.(*httpresponse.RequestWriter); ok && request.ErrorReference != "" {
				requestLogger = logger.With("error_reference", request.ErrorReference)
			}
			requestLogger.Info(
				"request",
				"event",
				"request_complete",
				"method",
				r.Method,
				"path",
				r.URL.Path,
				"duration",
				time.Since(started),
			)
		})
	}
}
