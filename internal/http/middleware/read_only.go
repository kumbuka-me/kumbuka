package middleware

import (
	"net/http"
	"strings"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
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
