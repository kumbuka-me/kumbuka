package pages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageReviewRequestInput contains the editable fields used to open a review.
type PageReviewRequestInput struct {
	// Slug identifies the page to review.
	Slug string
	// ReviewerUsernames selects individual reviewers by username.
	ReviewerUsernames []string
	// ReviewerGroupID optionally assigns a collaboration group as reviewer.
	ReviewerGroupID int64
	// Note is the request message shown to reviewers.
	Note string
	// Actor is the authenticated user opening the review.
	Actor domain.User
}

// PageReviewUpdateInput contains the editable fields of an existing pending review.
type PageReviewUpdateInput struct {
	// ID identifies the pending review request to update.
	ID int64
	// Slug binds the update to the expected page.
	Slug string
	// ReviewerUsernames replaces the individual reviewer set.
	ReviewerUsernames []string
	// ReviewerGroupID replaces the optional reviewer group.
	ReviewerGroupID int64
	// Note replaces the review request message.
	Note string
	// Actor is the authenticated user editing the request.
	Actor domain.User
}

// PageReviewDecisionInput contains one immutable decision for a pending review.
type PageReviewDecisionInput struct {
	// ID identifies the pending review request being decided.
	ID int64
	// Slug binds the decision to the expected page.
	Slug string
	// Decision is approved or changes_requested.
	Decision string
	// Note records the reviewer rationale.
	Note string
	// Actor is the authenticated reviewer making the decision.
	Actor domain.User
}

// pageReviewRepository contains persistence for review requests and reviewer resolution.
type pageReviewRepository interface {
	GetPage(context.Context, string) (domain.Page, error)
	PageReviewRequest(context.Context, string) (domain.PageReviewRequest, error)
	PageReviewRequestByID(context.Context, int64, string) (domain.PageReviewRequest, error)
	ReviewUsers(context.Context, []string) ([]domain.User, error)
	ReviewGroup(context.Context, int64) (domain.Group, error)
	ReviewGroups(context.Context) ([]domain.Group, error)
	RequestPageReview(context.Context, string, int64, []int64, int64, string) (domain.PageReviewRequest, error)
	UpdatePageReview(context.Context, int64, string, int64, []int64, int64, string) (domain.PageReviewRequest, error)
	CancelPageReview(context.Context, int64, string, int64) (string, error)
	CanReviewPage(context.Context, string, int64) (bool, error)
	DecidePageReview(context.Context, int64, string, int64, bool, string, string) (string, error)
}

// Reviews owns page approval workflows.
type Reviews struct {
	// repository persists review requests and reviewer decisions.
	repository pageReviewRepository
	// authorization applies page-level view and edit policy.
	authorization pageAuthorization
	// effects emits best-effort audit, notification, and webhook side effects.
	effects *pageEffects
}

// NewReviews constructs page review use cases.
func NewReviews(
	repository pageReviewRepository,
	access appaccess.Policy,
	sideEffects pageSideEffectRepository,
	logger *slog.Logger,
	eventSinks ...webhooks.EventSink,
) *Reviews {
	return &Reviews{
		repository:    repository,
		authorization: pageAuthorization{policy: access},
		effects:       newPageEffects(sideEffects, logger, eventSinks...),
	}
}

// PageReviewRequest returns the active review workflow item for a page.
func (s *Reviews) PageReviewRequest(ctx context.Context, slug string) (domain.PageReviewRequest, error) {
	return s.repository.PageReviewRequest(ctx, strings.TrimSpace(slug))
}

// ReviewGroups returns collaboration groups that can be selected as review targets.
func (s *Reviews) ReviewGroups(ctx context.Context) ([]domain.Group, error) {
	return s.repository.ReviewGroups(ctx)
}

// CanReview reports whether an editor is assigned to the current pending review.
func (s *Reviews) CanReview(ctx context.Context, slug string, actor domain.User) (bool, error) {
	if actor.IsAdministrator() {
		return true, nil
	}
	if actor.Role != domain.UserRoleEditor {
		return false, nil
	}

	return s.repository.CanReviewPage(ctx, strings.TrimSpace(slug), actor.ID)
}

// CanManageReview reports whether the actor may edit or cancel the pending request.
func (s *Reviews) CanManageReview(request domain.PageReviewRequest, actor domain.User) bool {
	if request.ID == 0 || request.Status != domain.PageReviewStatusPending {
		return false
	}

	return actor.IsAdministrator() || request.RequestedBy == actor.ID
}

// RequestReview opens a review for the current page revision and moves the page to draft.
func (s *Reviews) RequestReview(ctx context.Context, input PageReviewRequestInput) (domain.PageReviewRequest, error) {
	input.Slug = strings.TrimSpace(input.Slug)
	if input.Slug == "" {
		return domain.PageReviewRequest{}, domain.NewValidationError("slug", "A page path is required.")
	}
	if err := s.authorization.requireEdit(ctx, input.Actor, input.Slug); err != nil {
		return domain.PageReviewRequest{}, err
	}
	if !canRequestReview(input.Actor) {
		return domain.PageReviewRequest{}, domain.ErrForbidden
	}

	active, err := s.repository.PageReviewRequest(ctx, input.Slug)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}
	if active.Status == domain.PageReviewStatusPending {
		return domain.PageReviewRequest{}, domain.ErrReviewPending
	}
	if active.Status == domain.PageReviewStatusChangesRequested {
		return domain.PageReviewRequest{}, domain.ErrReviewChangesRequired
	}

	reviewerIDs, reviewerGroupID, err := s.resolveReviewTargets(
		ctx,
		input.Slug,
		input.ReviewerUsernames,
		input.ReviewerGroupID,
	)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	request, err := s.repository.RequestPageReview(
		ctx,
		input.Slug,
		input.Actor.ID,
		reviewerIDs,
		reviewerGroupID,
		strings.TrimSpace(input.Note),
	)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	s.effects.recordAudit(ctx, input.Actor.ID, "page.review_requested", "page", input.Slug, "Review requested for revision "+fmt.Sprint(request.RevisionNumber))
	s.effects.notifyWatchers(ctx, input.Actor.ID, input.Slug, "review-requested", "Review requested", "/pages/"+input.Slug)

	return request, nil
}

// UpdateReview changes reviewers, reviewer group, or note without changing the requested revision.
func (s *Reviews) UpdateReview(ctx context.Context, input PageReviewUpdateInput) (domain.PageReviewRequest, error) {
	if input.ID <= 0 {
		return domain.PageReviewRequest{}, domain.NewValidationError("review", "Choose a valid review request.")
	}
	input.Slug = strings.TrimSpace(input.Slug)
	if input.Slug == "" {
		return domain.PageReviewRequest{}, domain.NewValidationError("slug", "A page path is required.")
	}
	if err := s.authorization.requireEdit(ctx, input.Actor, input.Slug); err != nil {
		return domain.PageReviewRequest{}, err
	}

	request, err := s.repository.PageReviewRequestByID(ctx, input.ID, input.Slug)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}
	if request.Status != domain.PageReviewStatusPending {
		return domain.PageReviewRequest{}, domain.ErrReviewClosed
	}
	if !s.CanManageReview(request, input.Actor) {
		return domain.PageReviewRequest{}, domain.ErrForbidden
	}

	reviewerIDs, reviewerGroupID, err := s.resolveReviewTargets(
		ctx,
		input.Slug,
		input.ReviewerUsernames,
		input.ReviewerGroupID,
	)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	updated, err := s.repository.UpdatePageReview(
		ctx,
		input.ID,
		input.Slug,
		input.Actor.ID,
		reviewerIDs,
		reviewerGroupID,
		strings.TrimSpace(input.Note),
	)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	s.effects.recordAudit(ctx, input.Actor.ID, "page.review_updated", "page", input.Slug, "Pending review request updated")
	s.effects.notifyWatchers(ctx, input.Actor.ID, input.Slug, "review-updated", "Review request updated", "/pages/"+input.Slug)

	return updated, nil
}

// CancelReview cancels a pending request without rewriting its history.
func (s *Reviews) CancelReview(ctx context.Context, id int64, slug string, actor domain.User) error {
	if id <= 0 {
		return domain.NewValidationError("review", "Choose a valid review request.")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return domain.NewValidationError("slug", "A page path is required.")
	}
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return err
	}

	request, err := s.repository.PageReviewRequestByID(ctx, id, slug)
	if err != nil {
		return err
	}
	if request.Status != domain.PageReviewStatusPending {
		return domain.ErrReviewClosed
	}
	if !s.CanManageReview(request, actor) {
		return domain.ErrForbidden
	}

	resolvedSlug, err := s.repository.CancelPageReview(ctx, id, slug, actor.ID)
	if err != nil {
		return err
	}

	s.effects.recordAudit(ctx, actor.ID, "page.review_canceled", "page", resolvedSlug, "Pending review request canceled")
	s.effects.notifyWatchers(ctx, actor.ID, resolvedSlug, "review-canceled", "Review request canceled", "/pages/"+resolvedSlug)

	return nil
}

// validReviewDecision reports whether decision is one of the two reviewer outcomes.
func validReviewDecision(decision string) bool {
	return decision == domain.PageReviewStatusApproved || decision == domain.PageReviewStatusChangesRequested
}

// DecideReview approves the requested revision or asks the author for changes.
func (s *Reviews) DecideReview(ctx context.Context, input PageReviewDecisionInput) error {
	if input.ID <= 0 {
		return domain.NewValidationError("review", "Choose a valid review request.")
	}
	if !validReviewDecision(input.Decision) {
		return domain.NewValidationError("decision", "Choose approve or request changes.")
	}
	if err := s.authorization.requireView(ctx, input.Actor, input.Slug); err != nil {
		return err
	}

	allowed, err := s.CanReview(ctx, input.Slug, input.Actor)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	administrator := input.Actor.IsAdministrator()
	resolvedSlug, err := s.repository.DecidePageReview(
		ctx,
		input.ID,
		strings.TrimSpace(input.Slug),
		input.Actor.ID,
		administrator,
		input.Decision,
		strings.TrimSpace(input.Note),
	)
	if err != nil {
		return err
	}

	s.effects.recordAudit(ctx, input.Actor.ID, "page.review_"+input.Decision, "page", resolvedSlug, strings.TrimSpace(input.Note))
	s.effects.notifyWatchers(ctx, input.Actor.ID, resolvedSlug, "review-"+input.Decision, "Review "+strings.ReplaceAll(input.Decision, "_", " "), "/pages/"+resolvedSlug)

	return nil
}

// canRequestReview reports whether an actor may open a page review.
func canRequestReview(actor domain.User) bool {
	return actor.CanEditContent()
}

// resolveReviewTargets validates selected people and the optional group and applies the owner-group fallback.
func (s *Reviews) resolveReviewTargets(
	ctx context.Context,
	slug string,
	usernames []string,
	groupID int64,
) ([]int64, int64, error) {
	if groupID < 0 {
		return nil, 0, domain.NewValidationError("reviewer_group_id", "Choose a valid reviewer group.")
	}

	usernames = normalizeReviewerUsernames(usernames)
	reviewers, err := s.repository.ReviewUsers(ctx, usernames)
	if err != nil {
		return nil, 0, err
	}
	if len(reviewers) != len(usernames) || !validReviewers(reviewers) {
		return nil, 0, domain.NewValidationError("reviewers", "Choose enabled editors or administrators as reviewers.")
	}

	if groupID > 0 {
		if _, err := s.repository.ReviewGroup(ctx, groupID); errors.Is(err, domain.ErrNotFound) {
			return nil, 0, domain.NewValidationError("reviewer_group_id", "Choose an existing reviewer group.")
		} else if err != nil {
			return nil, 0, err
		}
	}

	if groupID == 0 && len(reviewers) == 0 {
		page, err := s.repository.GetPage(ctx, slug)
		if err != nil {
			return nil, 0, err
		}
		groupID = page.OwnerGroupID
	}

	ids := make([]int64, 0, len(reviewers))
	for _, reviewer := range reviewers {
		ids = append(ids, reviewer.ID)
	}

	return ids, groupID, nil
}

// validReviewers reports whether every selected account has a role that can decide reviews.
func validReviewers(reviewers []domain.User) bool {
	for _, reviewer := range reviewers {
		if !reviewer.CanEditContent() {
			return false
		}
	}

	return true
}

// normalizeReviewerUsernames trims mention markers, removes blanks, and keeps each username once.
func normalizeReviewerUsernames(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "@"))
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}

	return result
}
