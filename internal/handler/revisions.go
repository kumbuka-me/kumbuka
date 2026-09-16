package handler

import (
	"net/http"
	"strconv"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// RevisionHistory renders the full revision history fragment for a page.
func RevisionHistory(catalogUseCases pageRevisionService, views *Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		revisions, err := catalogUseCases.Revisions(r.Context(), r.PathValue("slug"))
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		user, _ := auth.User(r)

		renderFragment(views, w, "page", "revision-list", ViewData{
			Revisions:    revision.AnalyzeAll(revisions),
			RevisionSlug: r.PathValue("slug"),
			CanEdit:      user.Role == "admin" || user.Role == "editor",
		})
	}
}

// RestoreRevision creates a new page revision from an older persisted revision.
func RestoreRevision(pageUseCases pageRevisionWriter, views *Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		number, err := strconv.Atoi(r.PathValue("number"))
		if err != nil || number <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid revision.")
			return
		}

		page, err := pageUseCases.RestoreRevision(r.Context(), r.PathValue("slug"), number, user)
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+page.Slug, http.StatusSeeOther)
	}
}
