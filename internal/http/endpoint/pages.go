package endpoint

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
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
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := viewDataUseCases.Load(r, views, "Home")
		data := webview.HomeView{Layout: layout}
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
		data.HomeWidgets = webview.Widgets(widgets, "home", "", "/")

		views.Render(w, "home", data)
	}
}
