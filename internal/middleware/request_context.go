package middleware

import (
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"net/http"
)

// RequestContext supplies diagnostics even when access logging is disabled.
func RequestContext() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(httpresponse.NewRequestWriter(w, r), r)
		})
	}
}
