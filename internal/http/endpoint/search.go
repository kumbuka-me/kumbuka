package endpoint

import (
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"net/http"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// Search executes free-text search plus supported field filters.
func Search(
	viewDataUseCases viewDataService,
	catalogUseCases visiblePageSearchService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		pages, err := catalogUseCases.SearchFor(r.Context(), currentUser(r), query, 50)
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		layout, err := viewDataUseCases.Load(r, views, "Search")
		data := webview.SearchView{Layout: layout}
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.Query, data.Pages = query, pages

		data.SearchQuery = data.Query
		views.Render(w, "search", data)
	}
}
