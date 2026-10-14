package middleware

import (
	"net/http"
	"slices"
	"strings"

	appkithttpauth "github.com/containeroo/appkit/httpauth"
	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// RequireRole authorizes an authenticated request against one of the allowed roles.
func RequireRole(roles ...domain.UserRole) httpkitmiddleware.Middleware {
	allowed := slices.Clone(roles)

	return appkithttpauth.Authorize(
		func(_ *http.Request, user domain.User) bool { return slices.Contains(allowed, user.Role) },
		roleFailure,
	)
}

// roleFailure writes the application-specific response for an authorization denial.
func roleFailure(w http.ResponseWriter, r *http.Request, status int) {
	if status == http.StatusUnauthorized {
		roleUnauthorized(w, r)
		return
	}
	roleForbidden(w, r)
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
