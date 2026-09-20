package endpoint

import (
	"cmp"
	"context"
	"errors"
	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"html/template"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

// ViewPage renders one readable page and its active plugin detail widgets.
func ViewPage(
	viewDataUseCases viewDataService,
	catalogUseCases pageViewCatalogService,
	accessUseCases pageAccessReader,
	viewPage pageViewQuery,
	renderer *md.Renderer,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		r, timingTrace := views.StartPageTiming(r)
		defer views.LogPageTiming(timingTrace, r, slug)

		stop := measurePageStage(r.Context(), "page_lookup")
		user, _ := auth.User(r)
		result, err := viewPage.Execute(r.Context(), user, slug)
		page, alias := result.Page, result.Alias
		stop()
		if errors.Is(err, domain.ErrNotFound) {
			renderNotFoundPage(w, r, viewDataUseCases, views)
			return
		}
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}
		if alias != "" {
			http.Redirect(w, r, "/pages/"+alias, http.StatusPermanentRedirect)
			return
		}

		securedCatalog := apppages.NewAccessibleCatalog(catalogUseCases, accessUseCases, user)

		stop = measurePageStage(r.Context(), "record_view")
		apppages.RecordView(r.Context(), views.Logger(), catalogUseCases, slug, user.ID)
		stop()

		state, outgoingLinks := result.State, result.OutgoingLinks

		stop = measurePageStage(r.Context(), "view_data")
		layout, err := viewDataUseCases.Load(r, views, page.Title)
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
		pageNavigation := plugincap.Navigation(navigation.Children(data.Navigation, slug), pageURL)
		capabilities := plugincap.Capabilities(securedCatalog, pageNavigation, renderer.IconCatalog())
		stop()

		rendered, err := renderPageContent(r.Context(), page, md.DefaultOptions(), capabilities, renderer, catalogUseCases, views.Logger())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		stop = measurePageStage(r.Context(), "broken_links")
		renderedHTML := markBrokenWikiLinks(rendered.HTML, outgoingLinks)
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
		data.PageWatchScope = state.Watch.Scope
		data.PageContents = rendered.Contents
		data.HasPageContents = len(rendered.Contents) > 0
		if manager := renderer.PluginManager(); manager != nil {
			data.PluginPageActions = manager.PageActions(page.ID, page.Slug)
			data.PluginExporters = manager.Exporters(page.Slug)
		}

		stop = measurePageStage(r.Context(), "page_detail_widgets")
		pageValue := plugincap.PageValue(page)
		widgets, err := renderer.RenderWidgets(
			r.Context(),
			"page.details",
			&pageValue,
			data.PluginFeatures,
			capabilities,
			data.Preferences.HiddenPluginWidgets,
		)
		stop()
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.PageDetailWidgets = webview.Widgets(widgets, "page.details", page.Slug, pageURL(page.Slug))

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
