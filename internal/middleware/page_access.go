package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
)

type pageAccessPolicy interface {
	CanView(context.Context, domain.User, string) (bool, error)
	CanEdit(context.Context, domain.User, string) (bool, error)
}

// RequirePageView hides page routes denied by inherited path access rules.
func RequirePageView(access pageAccessPolicy) Middleware {
	return requirePageAccess(access, false)
}

// RequirePageEdit rejects mutations denied by inherited path access rules.
func RequirePageEdit(access pageAccessPolicy) Middleware {
	return requirePageAccess(access, true)
}

// requirePageAccess enforces inherited view or edit access for page routes.
func requirePageAccess(access pageAccessPolicy, edit bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := auth.User(r)
			if !ok {
				roleUnauthorized(w, r)
				return
			}

			path := strings.TrimSpace(r.PathValue("slug"))
			if path == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed, err := pageAccessAllowed(r.Context(), access, user, path, edit)
			if err != nil {
				httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
				return
			}
			if allowed {
				next.ServeHTTP(w, r)
				return
			}

			writePageAccessDenied(w, r, edit)
		})
	}
}

// pageAccessAllowed evaluates the requested access level for one page path.
func pageAccessAllowed(ctx context.Context, access pageAccessPolicy, user domain.User, path string, edit bool) (bool, error) {
	if edit {
		return access.CanEdit(ctx, user, path)
	}

	return access.CanView(ctx, user, path)
}

// writePageAccessDenied hides denied reads and rejects denied mutations.
func writePageAccessDenied(w http.ResponseWriter, r *http.Request, edit bool) {
	if edit {
		roleForbidden(w, r)
		return
	}

	httpresponse.Problem(w, http.StatusNotFound, "Page not found.")
}
