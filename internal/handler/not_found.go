package handler

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
)

// NotFound renders the themed browser 404 page.
func NotFound(viewDataUseCases viewDataService, views *Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderNotFoundPage(w, r, viewDataUseCases, views)
	}
}

// renderNotFoundPage renders a 404 response with the authenticated application layout.
func renderNotFoundPage(
	w http.ResponseWriter,
	r *http.Request,
	viewDataUseCases viewDataService,
	views *Views,
) {
	data, err := viewDataUseCases.Load(r, views, "Page not found")
	if err != nil {
		httpresponse.InternalServerError(views.logger, w, err)
		return
	}

	renderStatusPage(views, w, http.StatusNotFound, "layout", data, statusPagePresentation{
		Title:          "Page not found",
		Message:        "The page you are looking for does not exist or may have moved.",
		Icon:           "search-lucide",
		PrimaryLabel:   "Return home",
		PrimaryURL:     "/",
		PrimaryIcon:    "house-lucide",
		SecondaryLabel: "Search pages",
		SecondaryURL:   "/search",
		SecondaryIcon:  "search-lucide",
	})
}
