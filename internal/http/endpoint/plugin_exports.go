package endpoint

import (
	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"log/slog"
	"mime"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

// ExportPagePlugin invokes one active plugin exporter for an authorized page.
func ExportPagePlugin(
	catalog pageReportCatalogService,
	navigation navigationService,
	access pageAccessReader,
	renderer *markdown.Renderer,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizePageRequest(w, r, access, false) {
			return
		}
		manager := renderer.PluginManager()
		if manager == nil {
			httpresponse.Problem(w, http.StatusNotFound, "Plugin exporter not found.")
			return
		}

		slug := r.PathValue("slug")
		page, err := catalog.GetPage(r.Context(), slug)
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}
		user, _ := auth.User(r)
		securedCatalog := apppages.NewAccessibleCatalog(catalog, access, user)
		pageNavigation, err := subpageNavigation(r.Context(), navigation, access, user, slug)
		if err != nil {
			httpresponse.Problem(w, http.StatusInternalServerError, "Could not prepare plugin export.")
			return
		}

		file, err := manager.Export(
			r.Context(),
			r.PathValue("pluginID"),
			r.PathValue("moduleID"),
			plugin.Context{
				Capabilities: plugincap.Capabilities(securedCatalog, pageNavigation, renderer.IconCatalog()),
				Features:     manager.FeatureSettings(),
			},
			plugin.ExportRequest{Page: plugincap.PageValue(page), Source: page.Markdown},
		)
		if err != nil {
			httpresponse.Problem(w, http.StatusUnprocessableEntity, "Plugin export failed.")
			return
		}

		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Type", file.MediaType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename}))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(file.Data)
	}
}
