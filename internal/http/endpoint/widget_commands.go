package endpoint

import (
	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
	"github.com/kumbuka-me/sdk"
)

const maxWidgetCommandFormBytes = 16 << 10

// PluginWidgetCommand executes one host-mediated command from an active plugin widget.
func PluginWidgetCommand(
	catalog pageReportCatalogService,
	navigation navigationService,
	access pageAccessReader,
	renderer *md.Renderer,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		manager := renderer.PluginManager()
		if manager == nil {
			httpresponse.Problem(w, http.StatusNotFound, "Plugin widget not found.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxWidgetCommandFormBytes)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid widget command.")
			return
		}

		surface := r.PostForm.Get("surface")
		if !sdk.ValidWidgetSurface(surface) {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid widget surface.")
			return
		}
		pageSlug := r.PostForm.Get("page")
		if widgetSurfaceNeedsPage(surface) != (pageSlug != "") {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid widget page context.")
			return
		}

		user, _ := auth.User(r)
		var pageValue *sdk.Page
		capabilities := plugincap.Capabilities(nil, nil, renderer.IconCatalog())
		if slug := pageSlug; slug != "" {
			allowed, err := access.CanView(r.Context(), user, slug)
			if err != nil {
				httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
				return
			}
			if !allowed {
				httpresponse.Problem(w, http.StatusNotFound, "Page not found.")
				return
			}
			page, err := catalog.GetPage(r.Context(), slug)
			if err != nil {
				httpresponse.Problem(w, http.StatusNotFound, "Page not found.")
				return
			}
			value := plugincap.PageValue(page)
			pageValue = &value
			securedCatalog := apppages.NewAccessibleCatalog(catalog, access, user)
			pageNavigation, err := subpageNavigation(r.Context(), navigation, access, user, slug)
			if err != nil {
				httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
				return
			}
			capabilities = plugincap.Capabilities(securedCatalog, pageNavigation, renderer.IconCatalog())
		}

		result, err := manager.WidgetCommand(
			r.Context(),
			r.PathValue("pluginID"),
			r.PathValue("moduleID"),
			plugin.Context{Capabilities: capabilities, Features: manager.FeatureSettings()},
			plugin.WidgetCommandRequest{Surface: surface, Page: pageValue, Action: r.PathValue("actionID")},
		)
		if err != nil {
			httpresponse.Problem(w, http.StatusUnprocessableEntity, "Plugin command failed.")
			return
		}

		next := safeAuthNext(result.Redirect)
		if next == "" {
			next = safeAuthNext(r.PostForm.Get("next"))
		}
		if next == "" {
			next = "/"
		}
		http.Redirect(w, r, next, http.StatusSeeOther)
	}
}

// widgetSurfaceNeedsPage reports whether a widget surface is rendered with current-page context.
func widgetSurfaceNeedsPage(surface string) bool {
	switch surface {
	case "page.details", "page.after-content", "page.aside":
		return true
	default:
		return false
	}
}
