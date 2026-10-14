package middleware

import (
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// RequestContext supplies diagnostics even when access logging is disabled.
func RequestContext() httpkitmiddleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(httpresponse.NewRequestWriter(w, r), r)
		})
	}
}
