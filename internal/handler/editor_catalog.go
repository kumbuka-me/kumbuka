package handler

import (
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// EditorCatalog returns page and plugin-owned editor metadata used by editor intelligence.
func EditorCatalog(
	navigationUseCases navigationService,
	catalogUseCases pageAliasService,
	plugins *plugin.Manager,
	logger *slog.Logger,
) http.HandlerFunc {
	// pageItem supplies a link target and label for editor completion.
	type pageItem struct {
		// Slug is the canonical page path inserted into a link.
		Slug string `json:"slug"`
		// Title labels the completion in the editor.
		Title string `json:"title"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		pages, err := navigationUseCases.NavigationPages(r.Context())
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		items := make([]pageItem, 0, len(pages))
		for _, page := range pages {
			items = append(items, pageItem{Slug: page.Slug, Title: page.Title})
		}

		var completions []plugin.EditorCompletionItem
		var inserts []plugin.EditorInsertContribution
		if plugins != nil {
			completions, err = plugins.EditorCompletions(r.Context())
			if err != nil {
				httpresponse.InternalServerError(logger, w, err)
				return
			}
			inserts = plugins.EditorInserts()
		}

		aliases, err := catalogUseCases.PageAliases(r.Context())
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if aliases == nil {
			aliases = map[string]string{}
		}

		httpresponse.Respond(w, http.StatusOK, map[string]any{
			"pages":       items,
			"completions": jsonSlice(completions),
			"inserts":     jsonSlice(inserts),
			"aliases":     aliases,
		})
	}
}
