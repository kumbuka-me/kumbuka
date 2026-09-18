package handler

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// NotFound renders the themed browser 404 page.
func NotFound(viewDataUseCases viewDataService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderNotFoundPage(w, r, viewDataUseCases, views)
	}
}

// renderNotFoundPage renders a 404 response with the authenticated application layout.
func renderNotFoundPage(
	w http.ResponseWriter,
	r *http.Request,
	viewDataUseCases viewDataService,
	views *webview.Views,
) {
	data, err := viewDataUseCases.Load(r, views, "Page not found")
	if err != nil {
		httpresponse.InternalServerError(views.Logger(), w, err)
		return
	}

	renderStatusPage(views, w, http.StatusNotFound, "layout", data, notFoundPresentation())
}
