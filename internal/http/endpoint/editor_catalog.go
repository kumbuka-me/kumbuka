package endpoint

import (
	"context"
	"log/slog"
	"net/http"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// editorCatalogPage supplies a visible page link target and label for editor completion.
type editorCatalogPage struct {
	// Slug is the canonical page path inserted into a link.
	Slug string `json:"slug"`
	// Title labels the completion in the editor.
	Title string `json:"title"`
}

// editorCatalogPluginData contains plugin-owned metadata exposed to one editor actor.
type editorCatalogPluginData struct {
	// completions contains concrete plugin completion values.
	completions []plugin.EditorCompletionItem
	// completionProviders describes resource-backed completion providers allowed for the actor.
	completionProviders []plugin.EditorCompletionProvider
	// inserts contains plugin-contributed editor insertion commands.
	inserts []plugin.EditorInsertContribution
	// toolbar contains the resolved editor toolbar groups.
	toolbar []plugin.ToolbarGroup
	// widgets contains plugin-contributed editor widgets.
	widgets []plugin.EditorWidgetContribution
	// widgetProblems contains validation problems for plugin-contributed widgets.
	widgetProblems []plugin.EditorWidgetProblem
}

// EditorCatalog returns page and plugin-owned editor metadata used by editor intelligence.
func EditorCatalog(
	navigationUseCases navigationService,
	catalogUseCases pageAliasService,
	settingsUseCases settingsService,
	plugins *plugin.Manager,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		pages, err := navigationUseCases.VisiblePages(r.Context(), user)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		pluginData, err := loadEditorCatalogPluginData(r.Context(), user, settingsUseCases, plugins)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		aliases, err := catalogUseCases.PageAliasesFor(r.Context(), user)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if aliases == nil {
			aliases = map[string]string{}
		}

		httpresponse.Respond(w, http.StatusOK, map[string]any{
			"pages":                editorCatalogPages(pages),
			"completions":          jsonSlice(pluginData.completions),
			"completion_providers": jsonSlice(pluginData.completionProviders),
			"inserts":              jsonSlice(pluginData.inserts),
			"toolbar":              jsonSlice(pluginData.toolbar),
			"widgets":              jsonSlice(pluginData.widgets),
			"widget_problems":      jsonSlice(pluginData.widgetProblems),
			"aliases":              aliases,
		})
	}
}

// editorCatalogPages projects visible domain pages into the minimal editor completion shape.
func editorCatalogPages(pages []domain.Page) []editorCatalogPage {
	items := make([]editorCatalogPage, 0, len(pages))
	for _, page := range pages {
		items = append(items, editorCatalogPage{Slug: page.Slug, Title: page.Title})
	}
	return items
}

// loadEditorCatalogPluginData resolves plugin metadata and applies actor-specific creation capabilities.
func loadEditorCatalogPluginData(
	ctx context.Context,
	user domain.User,
	settingsUseCases settingsService,
	plugins *plugin.Manager,
) (editorCatalogPluginData, error) {
	if plugins == nil {
		return editorCatalogPluginData{}, nil
	}

	completions, err := plugins.EditorCompletions(ctx)
	if err != nil {
		return editorCatalogPluginData{}, err
	}

	settings, err := settingsUseCases.ApplicationSettings(ctx)
	if err != nil {
		return editorCatalogPluginData{}, err
	}

	widgets, widgetProblems := plugins.EditorWidgets()

	return editorCatalogPluginData{
		completions:         completions,
		completionProviders: scopeEditorCompletionProviders(plugins.EditorCompletionProviders(), user.IsAdministrator()),
		inserts:             plugins.EditorInserts(),
		toolbar:             plugins.ResolveEditorToolbar(settings.EditorToolbarOverrides),
		widgets:             widgets,
		widgetProblems:      widgetProblems,
	}, nil
}

// scopeEditorCompletionProviders removes resource-creation details unavailable to the current actor.
func scopeEditorCompletionProviders(
	providers []plugin.EditorCompletionProvider,
	canCreateResources bool,
) []plugin.EditorCompletionProvider {
	for index := range providers {
		providers[index].CanCreate = providers[index].CanCreate && canCreateResources
		if providers[index].CanCreate {
			continue
		}

		providers[index].ResourceID = ""
		providers[index].Replacement = ""
		providers[index].LabelField = ""
		providers[index].DetailField = ""
		providers[index].Fields = []plugin.EditorCompletionField{}
	}
	return providers
}
