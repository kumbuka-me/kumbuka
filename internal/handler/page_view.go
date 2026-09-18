package handler

import (
	"cmp"
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

// pageViewRecorder persists best-effort page-view activity.
type pageViewRecorder interface {
	RecordView(context.Context, string, int64) error
}

// recordPageView records activity without making page rendering depend on analytics persistence.
func recordPageView(ctx context.Context, logger *slog.Logger, recorder pageViewRecorder, slug string, userID int64) {
	if err := recorder.RecordView(ctx, slug, userID); err != nil && logger != nil {
		logger.ErrorContext(ctx,
			"record page view",
			"event", "page_view_record_failed",
			"slug", slug,
			"user_id", userID,
			"error", err,
		)
	}
}

// pageViewState contains user-specific page actions loaded before rendering a page.
type pageViewState struct {
	// favorite reports whether the current user has pinned the page.
	favorite bool
	// watch contains the current user's page-watch scope.
	watch domain.PageWatch
	// reviewRequest contains the active review workflow item, if any.
	reviewRequest domain.PageReviewRequest
	// canReview reports whether the current user may decide the active review.
	canReview bool
	// canEdit reports whether the current user may edit the page.
	canEdit bool
	// canManageReview reports whether the current user may update or cancel the active review.
	canManageReview bool
	// reviewGroups contains groups available as review targets.
	reviewGroups []domain.Group
}

// ViewPage renders one readable page and its active plugin detail widgets.
func ViewPage(
	viewDataUseCases viewDataService,
	catalogUseCases pageViewCatalogService,
	accessUseCases pageAccessReader,
	approvalUseCases pageApprovalService,
	renderer *md.Renderer,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		r, timingTrace := views.StartPageTiming(r)
		defer views.LogPageTiming(timingTrace, r, slug)

		stop := measurePageStage(r.Context(), "page_lookup")
		page, alias, err := getPageOrAlias(r.Context(), catalogUseCases, slug)
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

		user, _ := auth.User(r)
		securedCatalog := accessiblePageCatalog{catalog: catalogUseCases, access: accessUseCases, user: user}

		stop = measurePageStage(r.Context(), "record_view")
		recordPageView(r.Context(), views.Logger(), catalogUseCases, slug, user.ID)
		stop()

		state, err := loadPageViewState(r.Context(), slug, user, catalogUseCases, accessUseCases, approvalUseCases)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		stop = measurePageStage(r.Context(), "outgoing_links")
		outgoingLinks, err := catalogUseCases.PageLinks(r.Context(), slug)
		stop()
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		stop = measurePageStage(r.Context(), "view_data")
		data, err := viewDataUseCases.Load(r, views, page.Title)
		stop()
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		comments, err := loadPageComments(r.Context(), slug, data.ApplicationSettings.DiscussionsEnabled, catalogUseCases)
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
		data.PageReviewRequest = state.reviewRequest
		data.CanReviewPage = state.canReview
		data.CanManageReview = state.canManageReview
		data.ReviewGroups = state.reviewGroups
		data.CanEdit = state.canEdit
		data.PluginInspectors = rendered.Inspectors
		data.PluginExportFields = rendered.ExportFields
		data.Comments = comments
		data.PageFavorite = state.favorite
		data.PageWatchScope = state.watch.Scope
		data.PageContents = rendered.Contents
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
		views.Render(w, "page", data)
		stop()
	}
}

// loadPageViewState loads user-specific favorite, watch, edit, and review state for a page.
func loadPageViewState(
	ctx context.Context,
	slug string,
	user domain.User,
	catalog pageViewCatalogService,
	access pageAccessReader,
	approvals pageApprovalService,
) (pageViewState, error) {
	var state pageViewState

	stop := measurePageStage(ctx, "favorite_lookup")
	favorite, err := catalog.IsFavorite(ctx, slug, user.ID)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.favorite = favorite

	stop = measurePageStage(ctx, "page_watch")
	watch, err := catalog.PageWatch(ctx, slug, user.ID)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.watch = watch

	stop = measurePageStage(ctx, "review_request")
	reviewRequest, err := approvals.PageReviewRequest(ctx, slug)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.reviewRequest = reviewRequest

	stop = measurePageStage(ctx, "can_review")
	canReview, err := approvals.CanReview(ctx, slug, user)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.canReview = canReview

	stop = measurePageStage(ctx, "can_edit")
	canEdit, err := access.CanEdit(ctx, user, slug)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.canEdit = canEdit
	state.canManageReview = canEdit && approvals.CanManageReview(reviewRequest, user)

	if canEdit && (reviewRequest.ID == 0 || state.canManageReview) {
		stop = measurePageStage(ctx, "review_groups")
		state.reviewGroups, err = approvals.ReviewGroups(ctx)
		stop()
		if err != nil {
			return pageViewState{}, err
		}
	}

	return state, nil
}

// loadPageComments loads page discussions only when the application feature is enabled.
func loadPageComments(
	ctx context.Context,
	slug string,
	enabled bool,
	catalog pageViewCatalogService,
) ([]domain.PageComment, error) {
	if !enabled {
		return nil, nil
	}

	stop := measurePageStage(ctx, "comments")
	comments, err := catalog.PageComments(ctx, slug)
	stop()
	return comments, err
}

// getPageOrAlias resolves a page directly or returns the target of a matching alias.
func getPageOrAlias(
	ctx context.Context,
	catalogUseCases pageViewCatalogService,
	slug string,
) (domain.Page, string, error) {
	page, err := catalogUseCases.GetPage(ctx, slug)
	if err == nil {
		return page, "", nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Page{}, "", err
	}

	target, err := catalogUseCases.ResolvePageAlias(ctx, slug)
	if err != nil {
		return domain.Page{}, "", err
	}

	return domain.Page{}, target, nil
}
