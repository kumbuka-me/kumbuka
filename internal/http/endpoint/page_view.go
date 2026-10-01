package endpoint

import (
	"cmp"
	"context"
	"errors"
	"html/template"
	"net/http"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/route"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/pageurl"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
	"github.com/kumbuka-me/kumbuka/pkg/utils"
)

// ViewPage renders one readable page and its active plugin detail widgets.
func ViewPage(
	browserContext browserContextLoader,
	reports pageReportService,
	renderArtifacts pageRenderArtifactStore,
	viewPage pageViewQuery,
	renderer *md.Renderer,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		stop := measurePageStage(r.Context(), "page_lookup")
		user, _ := auth.User(r)
		result, err := viewPage.Execute(r.Context(), user, slug)
		page := result.Page
		stop()
		if errors.Is(err, domain.ErrNotFound) {
			renderNotFoundPage(w, r, browserContext, views)
			return
		}
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}
		securedCatalog := reports.Accessible(user)

		state, outgoingLinks := result.State, result.OutgoingLinks

		stop = measurePageStage(r.Context(), "view_data")
		layout, err := browserContext.Load(r, views, page.Title)
		data := webview.PageView{Layout: layout}
		data.PageContentLanguage = layout.ApplicationSettings.ContentLanguage
		stop()
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		comments, err := viewPage.Comments(r.Context(), page, data.ApplicationSettings.DiscussionsEnabled)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}
		data.PageContentLanguage = cmp.Or(page.Language, data.PageContentLanguage)

		stop = measurePageStage(r.Context(), "page_navigation")
		pageNavigation := plugincap.Navigation(navigation.Children(data.Navigation, slug), route.PrefixForRequest(r))
		capabilities := plugincap.Capabilities(securedCatalog, pageNavigation, route.PrefixForRequest(r), renderer.IconCatalog())
		stop()

		options := md.DefaultOptions()
		options.RoutePrefix = route.PrefixForRequest(r)
		rendered, err := renderPageContent(r.Context(), data.Locale.Code, page, outgoingLinks, options, capabilities, renderer, renderArtifacts, views.Logger())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		stop = measurePageStage(r.Context(), "broken_links")
		renderedHTML := markBrokenWikiLinks(rendered.HTML, outgoingLinks, options.RoutePrefix)
		stop()

		data.Page, data.HTML = &page, template.HTML(renderedHTML)
		data.PageReviewRequest = state.ReviewRequest
		data.CanReviewPage = state.CanReview
		data.CanManageReview = state.CanManageReview
		data.ReviewGroups = state.ReviewGroups
		data.CanEdit = state.CanEdit
		data.PluginInspectors = rendered.Inspectors
		data.PluginExportFields = rendered.ExportFields
		data.Comments, data.InlineCommentThreads = webview.PartitionPageComments(comments)
		data.PageFavorite = state.Favorite
		data.PageWatchScope = string(state.Watch.Scope)
		data.PageContents = rendered.Contents
		data.HasPageContents = len(rendered.Contents) > 0
		if manager := renderer.PluginManager(); manager != nil {
			data.PluginPageActions = manager.PageActions(page.ID, page.Slug)
			data.PluginExporters = manager.Exporters(page.Slug)
		}

		stop = measurePageStage(r.Context(), "page_detail_widgets")
		widgets, err := renderer.RenderWidgets(
			r.Context(),
			data.Locale.Code,
			"page.details",
			utils.ToPtr(plugincap.PageValue(page, route.PrefixForRequest(r))),
			data.PluginFeatures,
			capabilities,
			data.Preferences.HiddenPluginWidgets,
		)
		stop()
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.PageDetailWidgets = webview.Widgets(widgets, "page.details", page.Slug, pageurl.Page(page.ID, page.Slug))

		stop = measurePageStage(r.Context(), "template_render")
		data.CurrentPage = webview.CurrentPage(data.Page)
		views.Render(w, "page", data)
		stop()
	}
}

// pageViewQuery provides the authorized reading-page application result.
type pageViewQuery interface {
	Execute(context.Context, domain.User, string) (apppages.ViewResult, error)
	Comments(context.Context, domain.Page, bool) ([]domain.PageComment, error)
}
