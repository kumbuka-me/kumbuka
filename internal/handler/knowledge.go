package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// KnowledgeGraphPage renders the interactive page relationship explorer.
func KnowledgeGraphPage(viewDataUseCases viewDataService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := viewDataUseCases.Load(r, views, "Knowledge graph")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.Query = strings.TrimSpace(r.URL.Query().Get("slug"))

		views.Render(w, "graph", data)
	}
}

// KnowledgeGraphAPI returns pages and current wiki-link relationships.
func KnowledgeGraphAPI(knowledgeUseCases knowledgeGraphService, accessUseCases pageAccessReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		graph, err := knowledgeUseCases.KnowledgeGraph(r.Context(), 300)
		if err == nil {
			graph, err = visibleKnowledgeGraph(r.Context(), accessUseCases, currentUser(r), graph)
		}
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		graph.Nodes = jsonSlice(graph.Nodes)
		graph.Edges = jsonSlice(graph.Edges)
		httpresponse.Respond(w, http.StatusOK, graph)
	}
}
