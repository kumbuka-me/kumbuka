package pages

import (
	"context"
	"errors"
	"log/slog"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
)

// viewRepository contains the persistence needed for reading a page screen.
type viewRepository interface {
	GetPage(context.Context, string) (domain.Page, error)
	ResolvePageAlias(context.Context, string) (string, error)
	IsFavorite(context.Context, string, int64) (bool, error)
	PageWatch(context.Context, string, int64) (domain.PageWatch, error)
	PageLinks(context.Context, string) ([]domain.PageLink, error)
	PageComments(context.Context, string) ([]domain.PageComment, error)
	RecordView(context.Context, string, int64) error
}

// reviewReader supplies review state without coupling page queries to mutation machinery.
type reviewReader interface {
	PageReviewRequest(context.Context, string) (domain.PageReviewRequest, error)
	CanReview(context.Context, string, domain.User) (bool, error)
	CanManageReview(domain.PageReviewRequest, domain.User) bool
	ReviewGroups(context.Context) ([]domain.Group, error)
}

// View loads an authorized page and the actor's available actions.
type View struct {
	repository viewRepository
	access     accessReader
	reviews    reviewReader
	logger     *slog.Logger
}

// ViewResult contains application data for a reading page or an alias target.
type ViewResult struct {
	Page          domain.Page
	Alias         string
	State         ViewState
	OutgoingLinks []domain.PageLink
}

// NewView constructs the page reading query from its narrow read ports.
func NewView(repository viewRepository, access accessReader, reviews reviewReader, logger *slog.Logger) *View {
	return &View{repository: repository, access: access, reviews: reviews, logger: logger}
}

// Execute authorizes and loads one page, without recording view activity.
func (q *View) Execute(ctx context.Context, actor domain.User, slug string) (ViewResult, error) {
	if err := appaccess.RequireView(ctx, q.access, actor, slug); err != nil {
		return ViewResult{}, err
	}
	page, alias, err := getPageOrAlias(ctx, q.repository, slug)
	if err != nil {
		return ViewResult{}, err
	}
	if alias != "" {
		return ViewResult{Alias: alias}, nil
	}
	state, err := loadPageViewState(ctx, slug, actor, q.repository, q.access, q.reviews)
	if err != nil {
		return ViewResult{}, err
	}
	links, err := q.repository.PageLinks(ctx, slug)
	if err != nil {
		return ViewResult{}, err
	}
	stop := measurePageStage(ctx, "record_view")
	RecordView(ctx, q.logger, q.repository, page.Slug, actor.ID)
	stop()
	return ViewResult{Page: page, State: state, OutgoingLinks: links}, nil
}

// Comments loads discussions only when the shared feature setting enables them.
// The page comes from Execute, which has already authorized the resource.
func (q *View) Comments(ctx context.Context, page domain.Page, enabled bool) ([]domain.PageComment, error) {
	return loadPageComments(ctx, page.Slug, enabled, q.repository)
}

// measurePageStage records optional application timing without depending on HTTP.
func measurePageStage(ctx context.Context, stage string) func() {
	return renderprofile.FromContext(ctx).Measure(stage)
}

// pageViewRecorder persists best-effort page-view activity.
type pageViewRecorder interface {
	RecordView(context.Context, string, int64) error
}

// RecordView records activity without making page rendering depend on analytics persistence.
func RecordView(ctx context.Context, logger *slog.Logger, recorder pageViewRecorder, slug string, userID int64) {
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

// ViewState contains user-specific page actions loaded before rendering a page.
type ViewState struct {
	// Favorite reports whether the current user has pinned the page.
	Favorite bool
	// Watch contains the current user's page-Watch scope.
	Watch domain.PageWatch
	// ReviewRequest contains the active review workflow item, if any.
	ReviewRequest domain.PageReviewRequest
	// CanReview reports whether the current user may decide the active review.
	CanReview bool
	// CanEdit reports whether the current user may edit the page.
	CanEdit bool
	// CanManageReview reports whether the current user may update or cancel the active review.
	CanManageReview bool
	// ReviewGroups contains groups available as review targets.
	ReviewGroups []domain.Group
}

// loadPageViewState loads user-specific favorite, watch, edit, and review state for a page.
func loadPageViewState(
	ctx context.Context,
	slug string,
	user domain.User,
	catalog viewRepository,
	access accessReader,
	approvals reviewReader,
) (ViewState, error) {
	var state ViewState

	stop := measurePageStage(ctx, "favorite_lookup")
	favorite, err := catalog.IsFavorite(ctx, slug, user.ID)
	stop()
	if err != nil {
		return ViewState{}, err
	}
	state.Favorite = favorite

	stop = measurePageStage(ctx, "page_watch")
	watch, err := catalog.PageWatch(ctx, slug, user.ID)
	stop()
	if err != nil {
		return ViewState{}, err
	}
	state.Watch = watch

	stop = measurePageStage(ctx, "review_request")
	reviewRequest, err := approvals.PageReviewRequest(ctx, slug)
	stop()
	if err != nil {
		return ViewState{}, err
	}
	state.ReviewRequest = reviewRequest

	stop = measurePageStage(ctx, "can_review")
	canReview, err := approvals.CanReview(ctx, slug, user)
	stop()
	if err != nil {
		return ViewState{}, err
	}
	state.CanReview = canReview

	stop = measurePageStage(ctx, "can_edit")
	canEdit, err := access.CanEdit(ctx, user, slug)
	stop()
	if err != nil {
		return ViewState{}, err
	}
	state.CanEdit = canEdit
	state.CanManageReview = canEdit && approvals.CanManageReview(reviewRequest, user)

	if canEdit && (reviewRequest.ID == 0 || state.CanManageReview) {
		stop = measurePageStage(ctx, "review_groups")
		state.ReviewGroups, err = approvals.ReviewGroups(ctx)
		stop()
		if err != nil {
			return ViewState{}, err
		}
	}

	return state, nil
}

// loadPageComments loads page discussions only when the application feature is enabled.
func loadPageComments(
	ctx context.Context,
	slug string,
	enabled bool,
	catalog viewRepository,
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
	catalogUseCases viewRepository,
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
