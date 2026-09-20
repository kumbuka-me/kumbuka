package endpoint

import (
	"net/http"
	"strconv"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// RevisionHistory renders the full revision history fragment for a page.
func RevisionHistory(catalogUseCases visiblePageRevisionService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		revisions, err := catalogUseCases.RevisionsFor(r.Context(), currentUser(r), r.PathValue("slug"))
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		user, _ := auth.User(r)

		views.RenderFragment(w, "page", "revision-list", webview.RevisionsView{
			Revisions:    revision.AnalyzeAll(revisions),
			RevisionSlug: r.PathValue("slug"),
			Layout:       webview.Layout{CanEdit: user.CanEditContent()},
		})
	}
}

// RestoreRevision creates a new page revision from an older persisted revision.
func RestoreRevision(pageUseCases pageRevisionWriter, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		number, err := strconv.Atoi(r.PathValue("number"))
		if err != nil || number <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid revision.")
			return
		}

		page, err := pageUseCases.RestoreRevision(r.Context(), r.PathValue("slug"), number, user)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+page.Slug, http.StatusSeeOther)
	}
}
