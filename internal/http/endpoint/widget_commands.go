package endpoint

import (
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
	catalog pageReportService,
	navigation navigationService,
	renderer *md.Renderer,
	notifications plugincap.NotificationSender,
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
		pluginID := r.PathValue("pluginID")
		pluginName := activePluginName(manager, pluginID)
		if pluginName == "" {
			httpresponse.Problem(w, http.StatusNotFound, "Plugin widget not found.")
			return
		}
		var pageValue *sdk.Page
		capabilities := plugincap.MergeCapabilities(
			plugincap.Capabilities(nil, nil, renderer.IconCatalog()),
			plugincap.NotificationCapabilities(notifications, user.ID, pluginID, pluginName),
		)
		if slug := pageSlug; slug != "" {
			page, err := catalog.GetPageFor(r.Context(), user, slug)
			if err != nil {
				httpresponse.Problem(w, http.StatusNotFound, "Page not found.")
				return
			}
			value := plugincap.PageValue(page)
			pageValue = &value
			securedCatalog := catalog.Accessible(user)
			pageNavigation, err := subpageNavigation(r.Context(), navigation, user, slug)
			if err != nil {
				httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
				return
			}
			capabilities = plugincap.MergeCapabilities(
				plugincap.Capabilities(securedCatalog, pageNavigation, renderer.IconCatalog()),
				plugincap.NotificationCapabilities(notifications, user.ID, pluginID, pluginName),
			)
		}

		result, err := manager.WidgetCommand(
			r.Context(),
			pluginID,
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

// activePluginName returns the enabled plugin's host-validated display name.
func activePluginName(manager *plugin.Manager, pluginID string) string {
	for _, item := range manager.Plugins() {
		if item.Enabled && item.Manifest.ID == pluginID {
			return item.Manifest.Name
		}
	}
	return ""
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
