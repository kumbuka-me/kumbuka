package handler

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
)

// Search executes free-text search plus supported field filters.
func Search(
	viewDataUseCases viewDataService,
	catalogUseCases pageSearchService,
	accessUseCases pageAccessReader,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		pages, err := catalogUseCases.Search(r.Context(), query, 50)
		if err == nil {
			pages, err = accessUseCases.FilterPages(r.Context(), currentUser(r), pages)
		}
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data, err := viewDataUseCases.Load(r, views, "Search")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.Query, data.Pages = query, pages

		render(views, w, "search", data)
	}
}
