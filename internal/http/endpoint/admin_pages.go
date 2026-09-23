package endpoint

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// AdminPages renders bulk page management.
func AdminPages(
	browserContext browserContextLoader,
	catalogUseCases pageInventoryService,
	groupUseCases groupReader,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Pages", "pages")
		data := webview.AdminPagesView{Layout: layout}
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		pages, err := catalogUseCases.PageInventory(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.AdminPages = pages
		data.Groups = groups
		views.Render(w, "admin_pages", data)
	}
}

// BulkAdminPages applies one action to selected pages.
func BulkAdminPages(
	pageUseCases pageBulkService,
	catalogUseCases pageContentService,
	mediaUseCases portableArchiveExportMediaService,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid bulk page form.")
			return
		}

		slugs := uniqueNonEmpty(r.Form["slug"])
		if len(slugs) == 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Select at least one page.")
			return
		}

		action := r.FormValue("action")
		if action == "export" {
			file, modTime, cleanup, exportErr := createPortableExportArchive(r.Context(), catalogUseCases, mediaUseCases, slugs)
			if exportErr != nil {
				writePortableExportProblem(logger, w, exportErr)
				return
			}
			defer cleanup()

			filename := "kumbuka-pages-" + time.Now().UTC().Format("20060102-150405") + ".zip"
			serveExportArchive(w, r, filename, file, modTime)
			return
		}

		groupID := int64(0)
		var err error
		if action == "group" {
			groupID, err = parseRequiredPositiveFormInt64(r.FormValue("group_id"))
			if err != nil {
				httpresponse.Problem(w, http.StatusBadRequest, "Choose a valid group.")
				return
			}
		}

		if err := pageUseCases.Bulk(r.Context(), apppages.BulkPageInput{
			Action:  action,
			Slugs:   slugs,
			Status:  r.FormValue("status"),
			Tag:     r.FormValue("tag"),
			GroupID: groupID,
			Target:  r.FormValue("target"),
			Actor:   user,
		}); err != nil {
			writePageProblem(logger, w, err)
			return
		}

		http.Redirect(w, r, "/admin/pages", http.StatusSeeOther)
	}
}

// uniqueNonEmpty trims, deduplicates, and removes empty strings while preserving order.
func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}

		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}
