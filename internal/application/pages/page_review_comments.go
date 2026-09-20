package pages

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

const (
	maxReviewCommentBytes    = 8 * 1024
	maxReviewSuggestionBytes = 64 * 1024
	maxReviewAnchorLines     = 200
)

// PageReviewDetail contains one review snapshot plus its line feedback and actor permissions.
type PageReviewDetail struct {
	// Page is the current page associated with the review request.
	Page domain.Page
	// Request is the immutable review workflow item being inspected.
	Request domain.PageReviewRequest
	// Revision is the exact persisted revision requested for review.
	Revision revision.Revision
	// Comments contains line-anchored review comments and suggestions.
	Comments []domain.PageReviewComment
	// CanComment reports whether the actor may add discussion to the pending review.
	CanComment bool
	// CanSuggest reports whether the actor may propose applicable Markdown changes.
	CanSuggest bool
	// CanApply reports whether the actor may apply pending suggestions to the page.
	CanApply bool
}

// PageReviewCommentInput contains one line-anchored review comment or suggestion.
type PageReviewCommentInput struct {
	// ReviewID identifies the review request receiving feedback.
	ReviewID int64
	// Slug binds the feedback to the expected page.
	Slug string
	// Side selects the previous or reviewed side of the diff.
	Side string
	// StartLine is the first one-based source line covered by the feedback.
	StartLine int
	// EndLine is the last one-based source line covered by the feedback.
	EndLine int
	// Body is the optional explanatory review text.
	Body string
	// Suggestion reports whether Replacement is an applicable Markdown change.
	Suggestion bool
	// Replacement is the Markdown proposed for the selected reviewed lines.
	Replacement string
	// Actor is the authenticated user creating the feedback.
	Actor domain.User
}

// pageReviewDiscussionRepository contains persistence for review comments and atomic suggestion application.
type pageReviewDiscussionRepository interface {
	PageReviewComments(context.Context, int64) ([]domain.PageReviewComment, error)
	AddPageReviewComment(context.Context, int64, string, int64, string, int, int, string, bool, string, string) (domain.PageReviewComment, error)
	ApplyPageReviewSuggestions(context.Context, int64, string, int, int64, []int64, bool, string, string, []string, *pluginusage.Index, domain.PageRender) (domain.Page, error)
}

// reviewDiscussionRepository contains persistence required to inspect review snapshots and feedback.
type reviewDiscussionRepository interface {
	pageReviewDiscussionRepository
	PageReviewRequestByID(context.Context, int64, string) (domain.PageReviewRequest, error)
	GetPage(context.Context, string) (domain.Page, error)
	Revision(context.Context, string, int) (revision.Revision, error)
}

// reviewPolicy supplies approval permissions to review discussion use cases.
type reviewPolicy interface {
	CanReview(context.Context, string, domain.User) (bool, error)
	CanManageReview(domain.PageReviewRequest, domain.User) bool
}

// ReviewDiscussions owns line comments and applicable Markdown suggestions for reviews.
type ReviewDiscussions struct {
	repository    reviewDiscussionRepository
	authorization pageAuthorization
	reviews       reviewPolicy
	content       pageContentPreparer
	effects       *pageEffects
}

// NewReviewDiscussions constructs page review discussion use cases.
func NewReviewDiscussions(
	repository reviewDiscussionRepository,
	access appaccess.Policy,
	reviews reviewPolicy,
	content pageContentPreparer,
	sideEffects pageSideEffectRepository,
	logger *slog.Logger,
	eventSinks ...webhooks.EventSink,
) *ReviewDiscussions {
	return &ReviewDiscussions{
		repository:    repository,
		authorization: pageAuthorization{policy: access},
		reviews:       reviews,
		content:       content,
		effects:       newPageEffects(sideEffects, logger, eventSinks...),
	}
}

// WithContentPreparer uses the active Markdown preparation capability for suggestion application.
func (s *ReviewDiscussions) WithContentPreparer(preparer pageContentPreparer) *ReviewDiscussions {
	s.content = preparer
	return s
}

// ReviewDetail returns the requested revision, its feedback, and review permissions for the actor.
func (s *ReviewDiscussions) ReviewDetail(ctx context.Context, reviewID int64, slug string, actor domain.User) (PageReviewDetail, error) {
	slug = strings.TrimSpace(slug)
	if reviewID <= 0 || slug == "" {
		return PageReviewDetail{}, domain.ErrNotFound
	}
	if err := s.authorization.requireView(ctx, actor, slug); err != nil {
		return PageReviewDetail{}, err
	}

	request, err := s.repository.PageReviewRequestByID(ctx, reviewID, slug)
	if err != nil {
		return PageReviewDetail{}, err
	}
	page, err := s.repository.GetPage(ctx, slug)
	if err != nil {
		return PageReviewDetail{}, err
	}
	record, err := s.repository.Revision(ctx, slug, request.RevisionNumber)
	if err != nil {
		return PageReviewDetail{}, err
	}
	comments, err := s.repository.PageReviewComments(ctx, reviewID)
	if err != nil {
		return PageReviewDetail{}, err
	}

	pending := request.Status == domain.PageReviewStatusPending
	canManage := pending && s.reviews.CanManageReview(request, actor)
	canReview := false
	if pending {
		canReview, err = s.reviews.CanReview(ctx, slug, actor)
		if err != nil {
			return PageReviewDetail{}, err
		}
	}

	return PageReviewDetail{
		Page:       page,
		Request:    request,
		Revision:   record,
		Comments:   comments,
		CanComment: pending && (canReview || canManage),
		CanSuggest: pending && canReview,
		CanApply:   canManage,
	}, nil
}

// AddReviewComment validates and persists one comment or applicable suggestion against the immutable reviewed revision.
func (s *ReviewDiscussions) AddReviewComment(ctx context.Context, input PageReviewCommentInput) (domain.PageReviewComment, error) {
	input.Slug = strings.TrimSpace(input.Slug)
	input.Body = strings.TrimSpace(input.Body)
	input.Replacement = normalizeSuggestionText(input.Replacement)

	if err := validateReviewCommentInput(input); err != nil {
		return domain.PageReviewComment{}, err
	}

	detail, err := s.ReviewDetail(ctx, input.ReviewID, input.Slug, input.Actor)
	if err != nil {
		return domain.PageReviewComment{}, err
	}
	if detail.Request.Status != domain.PageReviewStatusPending {
		return domain.PageReviewComment{}, domain.ErrReviewClosed
	}
	if input.Suggestion && !detail.CanSuggest {
		return domain.PageReviewComment{}, domain.ErrForbidden
	}
	if !input.Suggestion && !detail.CanComment {
		return domain.PageReviewComment{}, domain.ErrForbidden
	}

	if !reviewRangeVisible(detail.Revision, input.Side, input.StartLine, input.EndLine) {
		return domain.PageReviewComment{}, domain.NewValidationError("line", "Choose a line that exists in this review diff.")
	}

	source := detail.Revision.Markdown
	if input.Side == domain.PageReviewCommentSideOld {
		source = detail.Revision.PreviousMarkdown
	}
	original, ok := markdownLineRange(source, input.StartLine, input.EndLine)
	if !ok {
		return domain.PageReviewComment{}, domain.NewValidationError("line", "Choose a line that exists in this review diff.")
	}

	comment, err := s.repository.AddPageReviewComment(
		ctx,
		input.ReviewID,
		input.Slug,
		input.Actor.ID,
		input.Side,
		input.StartLine,
		input.EndLine,
		input.Body,
		input.Suggestion,
		original,
		input.Replacement,
	)
	if err != nil {
		return domain.PageReviewComment{}, err
	}

	action := "page.review_commented"
	detailText := input.Body
	if input.Suggestion {
		action = "page.review_suggested"
		detailText = fmt.Sprintf("Suggested change for revision %d lines %d-%d", detail.Request.RevisionNumber, input.StartLine, input.EndLine)
	}
	s.effects.recordAudit(ctx, input.Actor.ID, action, "page", input.Slug, detailText)
	s.effects.notifyWatchers(ctx, input.Actor.ID, input.Slug, "Review feedback: "+detail.Page.Title, "New feedback was added to a page review.", reviewURL(input.ReviewID, input.Slug))

	return comment, nil
}

// ApplyReviewSuggestion applies one pending suggestion and creates a new page revision.
func (s *ReviewDiscussions) ApplyReviewSuggestion(ctx context.Context, reviewID int64, slug string, commentID int64, actor domain.User) (domain.Page, error) {
	if commentID <= 0 {
		return domain.Page{}, domain.NewValidationError("suggestion", "Choose a valid review suggestion.")
	}

	return s.applyReviewSuggestions(ctx, reviewID, slug, []int64{commentID}, actor)
}

// ApplyAllReviewSuggestions applies every pending suggestion in one atomic page revision.
func (s *ReviewDiscussions) ApplyAllReviewSuggestions(ctx context.Context, reviewID int64, slug string, actor domain.User) (domain.Page, error) {
	return s.applyReviewSuggestions(ctx, reviewID, slug, nil, actor)
}

// applyReviewSuggestions validates, merges, and persists selected suggestions as one new revision.
func (s *ReviewDiscussions) applyReviewSuggestions(ctx context.Context, reviewID int64, slug string, selected []int64, actor domain.User) (domain.Page, error) {
	applyAll := len(selected) == 0
	slug = strings.TrimSpace(slug)
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return domain.Page{}, err
	}
	detail, err := s.ReviewDetail(ctx, reviewID, slug, actor)
	if err != nil {
		return domain.Page{}, err
	}
	if detail.Request.Status != domain.PageReviewStatusPending {
		return domain.Page{}, domain.ErrReviewClosed
	}
	if !detail.CanApply {
		return domain.Page{}, domain.ErrForbidden
	}

	suggestions, err := selectedReviewSuggestions(detail.Comments, selected)
	if err != nil {
		return domain.Page{}, err
	}
	updatedMarkdown, err := applyMarkdownSuggestions(detail.Revision.Markdown, suggestions)
	if err != nil {
		return domain.Page{}, err
	}

	usage, render, err := preparePageContent(ctx, s.content, updatedMarkdown)
	if err != nil {
		return domain.Page{}, err
	}

	suggestionIDs := make([]int64, 0, len(suggestions))
	for _, suggestion := range suggestions {
		suggestionIDs = append(suggestionIDs, suggestion.ID)
	}
	message := "Apply review suggestion"
	if len(suggestions) > 1 {
		message = fmt.Sprintf("Apply %d review suggestions", len(suggestions))
	}

	page, err := s.repository.ApplyPageReviewSuggestions(
		ctx,
		reviewID,
		detail.Page.Slug,
		detail.Request.RevisionNumber,
		actor.ID,
		suggestionIDs,
		applyAll,
		updatedMarkdown,
		message,
		md.Links(updatedMarkdown),
		usage,
		render,
	)
	if err != nil {
		return domain.Page{}, err
	}

	s.effects.recordAudit(ctx, actor.ID, "page.review_suggestions_applied", "page", page.Slug, message)
	s.effects.notifyWatchers(ctx, actor.ID, page.Slug, "Review suggestions applied: "+page.Title, message+" and created a new revision.", "/pages/"+page.Slug)

	return page, nil
}

// validateReviewCommentInput checks bounded line feedback before loading review state.
func validateReviewCommentInput(input PageReviewCommentInput) error {
	validation := &domain.ValidationError{}

	if input.ReviewID <= 0 {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "review", Message: "Choose a valid review request."})
	}
	if input.Slug == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "slug", Message: "A page path is required."})
	}
	if input.Side != domain.PageReviewCommentSideOld && input.Side != domain.PageReviewCommentSideNew {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "side", Message: "Choose a valid side of the review diff."})
	}
	if input.StartLine <= 0 || input.EndLine < input.StartLine || input.EndLine-input.StartLine+1 > maxReviewAnchorLines {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "line", Message: "Choose a valid review line range."})
	}
	if len(input.Body) > maxReviewCommentBytes {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "body", Message: "Keep review comments below 8 KiB."})
	}
	if !input.Suggestion && input.Body == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "body", Message: "A review comment is required."})
	}
	if input.Suggestion && input.Side != domain.PageReviewCommentSideNew {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "suggestion", Message: "Suggestions can only replace lines in the reviewed revision."})
	}
	if input.Suggestion && len(input.Replacement) > maxReviewSuggestionBytes {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "replacement", Message: "Keep suggested Markdown below 64 KiB."})
	}

	if len(validation.Fields) > 0 {
		return validation
	}

	return nil
}

// reviewRangeVisible reports whether every source line in a requested range is present in the displayed review diff.
func reviewRangeVisible(record revision.Revision, side string, startLine, endLine int) bool {
	visible := make(map[int]struct{})
	for _, line := range revision.Analyze(record).Diff {
		lineNumber := line.NewLine
		if side == domain.PageReviewCommentSideOld {
			lineNumber = line.OldLine
		}
		if lineNumber > 0 {
			visible[lineNumber] = struct{}{}
		}
	}

	for lineNumber := startLine; lineNumber <= endLine; lineNumber++ {
		if _, ok := visible[lineNumber]; !ok {
			return false
		}
	}

	return startLine > 0 && endLine >= startLine
}

// selectedReviewSuggestions resolves an explicit subset or every open suggestion from the review comments.
func selectedReviewSuggestions(comments []domain.PageReviewComment, selected []int64) ([]domain.PageReviewComment, error) {
	selectedSet := make(map[int64]struct{}, len(selected))
	for _, id := range selected {
		if id <= 0 {
			return nil, domain.NewValidationError("suggestion", "Choose valid review suggestions.")
		}
		selectedSet[id] = struct{}{}
	}

	all := len(selectedSet) == 0
	result := make([]domain.PageReviewComment, 0, len(comments))
	for _, comment := range comments {
		if !comment.IsSuggestion || comment.AppliedAt != nil {
			continue
		}
		if all {
			result = append(result, comment)
			continue
		}
		if _, ok := selectedSet[comment.ID]; ok {
			result = append(result, comment)
			delete(selectedSet, comment.ID)
		}
	}

	if len(selectedSet) != 0 {
		return nil, domain.ErrNotFound
	}
	if len(result) == 0 {
		return nil, domain.NewValidationError("suggestion", "There are no pending review suggestions to apply.")
	}

	return result, nil
}

// applyMarkdownSuggestions applies non-overlapping line replacements from bottom to top against an immutable source snapshot.
func applyMarkdownSuggestions(markdown string, suggestions []domain.PageReviewComment) (string, error) {
	ordered := slices.Clone(suggestions)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].StartLine != ordered[right].StartLine {
			return ordered[left].StartLine < ordered[right].StartLine
		}
		return ordered[left].EndLine < ordered[right].EndLine
	})

	previousEnd := 0
	for _, suggestion := range ordered {
		if suggestion.Side != domain.PageReviewCommentSideNew || suggestion.StartLine <= previousEnd {
			return "", domain.ErrReviewSuggestionConflict
		}
		original, ok := markdownLineRange(markdown, suggestion.StartLine, suggestion.EndLine)
		if !ok || original != suggestion.Original {
			return "", domain.ErrStaleReview
		}
		previousEnd = suggestion.EndLine
	}

	lines := strings.Split(markdown, "\n")
	for index := len(ordered) - 1; index >= 0; index-- {
		suggestion := ordered[index]
		start := suggestion.StartLine - 1
		end := suggestion.EndLine
		replacement := suggestionLines(suggestion.Replacement)

		updated := make([]string, 0, len(lines)-(end-start)+len(replacement))
		updated = append(updated, lines[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[end:]...)
		lines = updated
	}

	return strings.Join(lines, "\n"), nil
}

// markdownLineRange returns an exact one-based inclusive source range without trailing line separators.
func markdownLineRange(markdown string, startLine, endLine int) (string, bool) {
	if startLine <= 0 || endLine < startLine {
		return "", false
	}

	lines := strings.Split(markdown, "\n")
	if startLine > len(lines) || endLine > len(lines) {
		return "", false
	}

	return strings.Join(lines[startLine-1:endLine], "\n"), true
}

// suggestionLines converts replacement Markdown into source lines, treating an empty replacement as deletion.
func suggestionLines(replacement string) []string {
	if replacement == "" {
		return nil
	}

	return strings.Split(replacement, "\n")
}

// reviewURL returns the local review page URL for notifications.
func reviewURL(reviewID int64, slug string) string {
	return fmt.Sprintf("/reviews/%d/%s", reviewID, strings.TrimSpace(slug))
}
