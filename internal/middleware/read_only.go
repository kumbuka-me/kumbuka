package middleware

import (
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
)

// ReadOnly blocks state-changing application requests while keeping authentication flows available.
func ReadOnly() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) || strings.HasPrefix(r.URL.Path, "/auth/") {
				next.ServeHTTP(w, r)
				return
			}

			httpresponse.Problem(w, http.StatusServiceUnavailable, "Kumbuka is running in read-only mode.")
		})
	}
}
