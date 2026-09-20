package endpoint

import (
	"errors"
	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"net/http"
	"strings"
)

// authorizePageRequest translates application access failures at the endpoint boundary.
// Authentication and coarse roles have already been handled by HTTP middleware.
func authorizePageRequest(w http.ResponseWriter, r *http.Request, policy appaccess.Policy, edit bool) bool {
	var err error
	if edit {
		err = appaccess.RequireEdit(r.Context(), policy, currentUser(r), r.PathValue("slug"))
	} else {
		err = appaccess.RequireView(r.Context(), policy, currentUser(r), r.PathValue("slug"))
	}
	switch {
	case err == nil:
		return true
	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w, http.StatusNotFound, "Page not found.")
	case errors.Is(err, domain.ErrForbidden):
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httpresponse.Problem(w, http.StatusForbidden, "Forbidden.", httpresponse.NewFieldProblem("authorization", "Your account does not have permission to perform this action."))
		} else {
			httpresponse.Problem(w, http.StatusForbidden, "Forbidden.")
		}
	default:
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
	}
	return false
}
