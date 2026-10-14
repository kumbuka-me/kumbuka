package middleware

import (
	"log/slog"
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// AccessLog records request method, path, and elapsed time after handling.
func AccessLog(logger *slog.Logger) httpkitmiddleware.Middleware {
	return httpkitmiddleware.AccessLogWithAttributes(logger, func(w http.ResponseWriter, _ *http.Request) []any {
		request, ok := w.(*httpresponse.RequestWriter)
		if !ok || request.ErrorReference == "" {
			return nil
		}
		return []any{"error_reference", request.ErrorReference}
	})
}
