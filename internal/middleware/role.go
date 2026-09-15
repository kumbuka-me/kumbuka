package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
)

// RequireRole authorizes an authenticated request against one of the allowed roles.
func RequireRole(roles ...string) Middleware {
	allowed := slices.Clone(roles)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := auth.User(r)
			if !ok {
				roleUnauthorized(w, r)
				return
			}
			if !slices.Contains(allowed, user.Role) {
				roleForbidden(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// roleUnauthorized writes a protocol-appropriate response when authentication middleware was omitted.
func roleUnauthorized(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		httpresponse.Problem(w,
			http.StatusUnauthorized,
			"Unauthorized.",
			httpresponse.NewFieldProblem("authorization", "An authenticated user is required."),
		)
		return
	}
	httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
}

// roleForbidden writes a protocol-appropriate response when the current role is not allowed.
func roleForbidden(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		httpresponse.Problem(w,
			http.StatusForbidden,
			"Forbidden.",
			httpresponse.NewFieldProblem(
				"authorization",
				"Your account does not have permission to perform this action.",
			),
		)
		return
	}
	httpresponse.Problem(w, http.StatusForbidden, "Forbidden.")
}
