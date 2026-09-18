package handler

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

// Home renders the dashboard for the current user.
func Home(
	viewDataUseCases viewDataService,
	catalogUseCases homeCatalogService,
	draftUseCases draftListService,
	accessUseCases pageAccessReader,
	renderer *md.Renderer,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := viewDataUseCases.Load(r, views, "Home")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		user, _ := auth.User(r)
		source := homeWidgetSource{catalog: catalogUseCases, drafts: draftUseCases, access: accessUseCases, user: user}
		capabilities := plugincap.MergeCapabilities(
			plugincap.Capabilities(nil, nil, renderer.IconCatalog()),
			plugincap.PageListCapabilities(source),
			plugincap.DraftCapabilities(source),
		)
		widgets, err := renderer.RenderWidgets(r.Context(), "home", nil, data.PluginFeatures, capabilities, data.Preferences.HiddenPluginWidgets)
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.HomeWidgets = widgetViews(widgets, "home", "", "/")

		render(views, w, "home", data)
	}
}
