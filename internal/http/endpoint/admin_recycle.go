package endpoint

import (
	"log/slog"
	"net/http"
	"strings"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// AdminBin renders pages that have been moved to the recycle bin.
func AdminBin(
	viewDataUseCases viewDataService,
	recycleBinUseCases recycleBinService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Recycle bin", "bin")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		pages, err := recycleBinUseCases.DeletedPages(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.DeletedPages = pages

		views.Render(w, "admin_bin", data)
	}
}

// RestoreAdminPage restores one page from the recycle bin.
func RestoreAdminPage(recycleBinUseCases recycleBinService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSpace(r.PathValue("slug"))
		if slug == "" {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"A page path is required.",
				httpresponse.NewFieldProblem("slug", "Choose a page to restore."),
			)
			return
		}
		if err := recycleBinUseCases.RestorePage(r.Context(), slug); err != nil {
			writeAdminProblem(logger, w, err, "Page")
			return
		}

		http.Redirect(w, r, "/admin/bin", http.StatusSeeOther)
	}
}

// PermanentlyDeleteAdminPage removes one page from the recycle bin permanently.
func PermanentlyDeleteAdminPage(recycleBinUseCases recycleBinService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSpace(r.PathValue("slug"))
		if slug == "" {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"A page path is required.",
				httpresponse.NewFieldProblem("slug", "Choose a page to delete permanently."),
			)
			return
		}
		if err := recycleBinUseCases.PermanentlyDeletePage(r.Context(), slug); err != nil {
			writeAdminProblem(logger, w, err, "Page")
			return
		}

		http.Redirect(w, r, "/admin/bin", http.StatusSeeOther)
	}
}
