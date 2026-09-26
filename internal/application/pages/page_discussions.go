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
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

const (
	maxInlineSuggestionAnchorBytes = 8 * 1024
	maxInlineSuggestionBodyBytes   = 8 * 1024
	maxInlineSuggestionBytes       = 64 * 1024
)

// pageDiscussionRepository contains discussion persistence and reply notification operations.
type pageDiscussionRepository interface {
	AddPageComment(context.Context, string, int64, int64, string, string, string) (domain.PageComment, error)
	AddPageSuggestion(context.Context, string, int64, string, string, domain.PageCommentSuggestion, string) (domain.PageComment, error)
	PageComment(context.Context, string, int64) (domain.PageComment, error)
	ApplyPageCommentSuggestion(context.Context, string, int64, int64, string, string, []string, *pluginusage.Index, domain.PageRender) (domain.Page, error)
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	NotifyCommentReply(context.Context, int64, int64, string, string) error
	ResolvePageComment(context.Context, string, int64, bool) error
}

// discussionRepository composes only persistence required by page discussions.
type discussionRepository interface {
	pageDiscussionRepository
	GetPage(context.Context, string) (domain.Page, error)
	LatestRevision(context.Context, string) (revision.Revision, int, error)
}

// Discussions owns page comments and inline Markdown suggestions.
type Discussions struct {
	// repository persists page discussions and loads suggestion context.
	repository discussionRepository
	// authorization applies page-level view and edit policy.
	authorization pageAuthorization
	// content prepares replacement Markdown before a suggestion is applied.
	content pageContentPreparer
	// effects emits best-effort audit, notification, and webhook side effects.
	effects *pageEffects
	// logger records non-fatal discussion notification failures.
	logger *slog.Logger
}

// NewDiscussions constructs page discussion use cases.
func NewDiscussions(
	repository discussionRepository,
	access appaccess.Policy,
	content pageContentPreparer,
	sideEffects pageSideEffectRepository,
	logger *slog.Logger,
	eventSinks ...webhooks.EventSink,
) *Discussions {
	if logger == nil {
		logger = slog.Default()
	}
	return &Discussions{
		repository:    repository,
		authorization: pageAuthorization{policy: access},
		content:       content,
		effects:       newPageEffects(sideEffects, logger, eventSinks...),
		logger:        logger,
	}
}

// WithMentionNotifications routes discussion mentions through the shared notification service.
func (s *Discussions) WithMentionNotifications(sender MentionNotificationSender) *Discussions {
	if s.effects != nil {
		s.effects.withMentionNotifications(sender)
	}
	return s
}

// ErrDiscussionsDisabled indicates that page discussions are globally disabled.
var ErrDiscussionsDisabled = errors.New("page discussions are disabled")

// WithContentPreparer uses the active Markdown preparation capability for suggestion application.
func (s *Discussions) WithContentPreparer(preparer pageContentPreparer) *Discussions {
	s.content = preparer
	return s
}

// AddComment adds a discussion comment and emits mention notifications.
func (s *Discussions) AddComment(
	ctx context.Context,
	slug string,
	parentID int64,
	anchor, quote, body string,
	actor domain.User,
) (domain.PageComment, error) {
	body = strings.TrimSpace(body)
	if err := s.authorization.requireView(ctx, actor, slug); err != nil {
		return domain.PageComment{}, err
	}
	if body == "" {
		return domain.PageComment{}, domain.NewValidationError("body", "A comment is required.")
	}
	if err := s.requireDiscussions(ctx); err != nil {
		return domain.PageComment{}, err
	}

	slug = strings.TrimSpace(slug)
	comment, err := s.repository.AddPageComment(ctx, slug, actor.ID, parentID, anchor, quote, body)
	if err != nil {
		return domain.PageComment{}, err
	}

	destination := pageCommentURL(slug, comment.ID)
	s.effects.notifyMentions(ctx, actor.ID, body, "Mention in "+slug, destination)
	if parentID > 0 {
		if err := s.repository.NotifyCommentReply(ctx, actor.ID, parentID, "Reply in "+slug, destination); err != nil {
			s.logger.ErrorContext(ctx, "comment reply notification failed", "event", "page_side_effect_failed", "error", err)
		}
	}
	s.effects.notifyWatchers(ctx, actor.ID, slug, "New comment: "+slug, "A watched page has a new discussion comment.", destination)
	s.effects.recordAudit(ctx, actor.ID, "comment.created", "page", slug, "Page discussion comment created")

	return comment, nil
}

// AddSuggestion adds an inline Markdown suggestion anchored to uniquely mapped selected page text.
func (s *Discussions) AddSuggestion(
	ctx context.Context,
	slug, anchor, body, replacement string,
	actor domain.User,
) (domain.PageComment, error) {
	slug = strings.TrimSpace(slug)
	if err := s.authorization.requireView(ctx, actor, slug); err != nil {
		return domain.PageComment{}, err
	}
	anchor = strings.TrimSpace(anchor)
	body = strings.TrimSpace(body)
	replacement = normalizeSuggestionText(replacement)

	if err := validateInlineSuggestionInput(slug, anchor, body, replacement); err != nil {
		return domain.PageComment{}, err
	}
	if err := s.requireDiscussions(ctx); err != nil {
		return domain.PageComment{}, err
	}

	page, err := s.repository.GetPage(ctx, slug)
	if err != nil {
		return domain.PageComment{}, err
	}
	latest, count, err := s.repository.LatestRevision(ctx, slug)
	if err != nil {
		return domain.PageComment{}, err
	}
	if !revisionMatchesPage(latest, count, page.Markdown) {
		return domain.PageComment{}, domain.ErrStaleSuggestion
	}

	start, end, err := locateInlineSuggestionSource(page.Markdown, anchor)
	if err != nil {
		return domain.PageComment{}, err
	}
	original := page.Markdown[start:end]
	if replacement == original {
		return domain.PageComment{}, domain.NewValidationError("replacement", "Change the selected text before creating a suggestion.")
	}

	suggestion := domain.PageCommentSuggestion{
		RevisionNumber: latest.Number,
		StartByte:      start,
		EndByte:        end,
		Original:       original,
		Replacement:    replacement,
	}
	comment, err := s.repository.AddPageSuggestion(ctx, slug, actor.ID, anchor, body, suggestion, page.Markdown)
	if err != nil {
		return domain.PageComment{}, err
	}

	destination := pageCommentURL(slug, comment.ID)
	if body != "" {
		s.effects.notifyMentions(ctx, actor.ID, body, "Mention in "+slug, destination)
	}
	s.effects.notifyWatchers(ctx, actor.ID, slug, "New suggestion: "+slug, "A watched page has a new inline suggestion.", destination)
	s.effects.recordAudit(ctx, actor.ID, "comment.suggested", "page", slug, "Inline Markdown suggestion created")

	return comment, nil
}

// ApplyCommentSuggestion applies one still-current inline suggestion and creates a new page revision.
func (s *Discussions) ApplyCommentSuggestion(
	ctx context.Context,
	slug string,
	commentID int64,
	actor domain.User,
) (domain.Page, error) {
	slug = strings.TrimSpace(slug)
	if err := s.authorizeCommentSuggestion(ctx, slug, commentID, actor); err != nil {
		return domain.Page{}, err
	}

	comment, page, err := s.loadApplicableCommentSuggestion(ctx, slug, commentID)
	if err != nil {
		return domain.Page{}, err
	}
	updatedMarkdown, err := applyInlineSuggestion(page.Markdown, *comment.Suggestion)
	if err != nil {
		return domain.Page{}, err
	}
	usage, render, err := preparePageContent(ctx, s.content, updatedMarkdown)
	if err != nil {
		return domain.Page{}, err
	}

	message := fmt.Sprintf("Apply inline suggestion from comment #%d", comment.ID)
	updated, err := s.repository.ApplyPageCommentSuggestion(
		ctx, slug, comment.ID, actor.ID, updatedMarkdown, message, md.Links(updatedMarkdown), usage, render,
	)
	if err != nil {
		return domain.Page{}, err
	}
	s.recordAppliedCommentSuggestion(ctx, actor, updated, comment.ID, message)
	return updated, nil
}

// authorizeCommentSuggestion validates access, identifiers, actor role, and discussion availability.
func (s *Discussions) authorizeCommentSuggestion(ctx context.Context, slug string, commentID int64, actor domain.User) error {
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return err
	}
	if slug == "" || commentID <= 0 {
		return domain.NewValidationError("suggestion", "Choose a valid inline suggestion.")
	}
	if !canApplyInlineSuggestion(actor) {
		return domain.ErrForbidden
	}
	return s.requireDiscussions(ctx)
}

// loadApplicableCommentSuggestion loads and validates a still-current inline suggestion.
func (s *Discussions) loadApplicableCommentSuggestion(ctx context.Context, slug string, commentID int64) (domain.PageComment, domain.Page, error) {
	comment, err := s.repository.PageComment(ctx, slug, commentID)
	if err != nil {
		return domain.PageComment{}, domain.Page{}, err
	}
	if comment.Suggestion == nil {
		return domain.PageComment{}, domain.Page{}, domain.NewValidationError("suggestion", "This comment does not contain an applicable suggestion.")
	}
	if comment.Suggestion.AppliedAt != nil {
		return domain.PageComment{}, domain.Page{}, domain.NewValidationError("suggestion", "This suggestion has already been applied.")
	}

	page, err := s.repository.GetPage(ctx, slug)
	if err != nil {
		return domain.PageComment{}, domain.Page{}, err
	}
	latest, count, err := s.repository.LatestRevision(ctx, slug)
	if err != nil {
		return domain.PageComment{}, domain.Page{}, err
	}
	if !suggestionMatchesRevision(*comment.Suggestion, latest, count, page.Markdown) {
		return domain.PageComment{}, domain.Page{}, domain.ErrStaleSuggestion
	}
	return comment, page, nil
}

// recordAppliedCommentSuggestion emits audit and watcher side effects after persistence succeeds.
func (s *Discussions) recordAppliedCommentSuggestion(ctx context.Context, actor domain.User, updated domain.Page, commentID int64, message string) {
	s.effects.recordAudit(ctx, actor.ID, "comment.suggestion_applied", "page", updated.Slug, message)
	s.effects.notifyWatchers(ctx, actor.ID, updated.Slug, "Inline suggestion applied: "+updated.Title, message+" and created a new revision.", pageCommentURL(updated.Slug, commentID))
}

// ResolveComment changes one page-bound discussion's resolution state.
func (s *Discussions) ResolveComment(ctx context.Context, slug string, id int64, resolved bool, actor domain.User) error {
	slug = strings.TrimSpace(slug)
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return err
	}
	if slug == "" {
		return &domain.ValidationError{Fields: []domain.FieldError{{Field: "slug", Message: "A page path is required."}}}
	}
	if id <= 0 {
		return &domain.ValidationError{Fields: []domain.FieldError{{Field: "comment", Message: "Invalid comment."}}}
	}
	if err := s.requireDiscussions(ctx); err != nil {
		return err
	}

	return s.repository.ResolvePageComment(ctx, slug, id, resolved)
}

// requireDiscussions rejects discussion mutations while the global feature is disabled.
func (s *Discussions) requireDiscussions(ctx context.Context) error {
	settings, err := s.repository.ApplicationSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.DiscussionsEnabled {
		return ErrDiscussionsDisabled
	}

	return nil
}

// validateInlineSuggestionInput validates bounded browser input before source mapping.
func validateInlineSuggestionInput(slug, anchor, body, replacement string) error {
	validation := &domain.ValidationError{}
	if slug == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "slug", Message: "A page path is required."})
	}
	if anchor == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "anchor", Message: "Select text on the page before creating a suggestion."})
	} else if len(anchor) > maxInlineSuggestionAnchorBytes {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "anchor", Message: "Select a shorter passage before creating a suggestion."})
	}
	if len(body) > maxInlineSuggestionBodyBytes {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "body", Message: "Keep suggestion comments below 8 KiB."})
	}
	if len(replacement) > maxInlineSuggestionBytes {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "replacement", Message: "Keep suggested Markdown below 64 KiB."})
	}

	if len(validation.Fields) > 0 {
		return validation
	}

	return nil
}

// locateInlineSuggestionSource maps selected rendered text only when it occurs exactly once in canonical Markdown.
func locateInlineSuggestionSource(markdown, anchor string) (start, end int, err error) {
	start = strings.Index(markdown, anchor)
	if start < 0 {
		return 0, 0, domain.NewValidationError(
			"anchor",
			"The selected text cannot be mapped exactly to the Markdown source. Select plain text without rendered formatting.",
		)
	}

	if strings.LastIndex(markdown, anchor) != start {
		return 0, 0, domain.NewValidationError(
			"anchor",
			"The selected text appears more than once in the Markdown source. Select a more specific passage.",
		)
	}

	return start, start + len(anchor), nil
}

// applyInlineSuggestion replaces one exact byte range after verifying its original Markdown source.
func applyInlineSuggestion(markdown string, suggestion domain.PageCommentSuggestion) (string, error) {
	if !validSuggestionRange(suggestion, len(markdown)) {
		return "", domain.ErrStaleSuggestion
	}
	if markdown[suggestion.StartByte:suggestion.EndByte] != suggestion.Original {
		return "", domain.ErrStaleSuggestion
	}

	return markdown[:suggestion.StartByte] + suggestion.Replacement + markdown[suggestion.EndByte:], nil
}

// revisionMatchesPage reports whether the latest persisted revision matches the current page Markdown.
func revisionMatchesPage(latest revision.Revision, count int, markdown string) bool {
	return count > 0 && latest.Number > 0 && latest.Markdown == markdown
}

// suggestionMatchesRevision reports whether a suggestion still targets the current page revision.
func suggestionMatchesRevision(suggestion domain.PageCommentSuggestion, latest revision.Revision, count int, markdown string) bool {
	return count > 0 && latest.Number == suggestion.RevisionNumber && latest.Markdown == markdown
}

// validSuggestionRange reports whether a suggestion selects a non-empty byte range inside the source.
func validSuggestionRange(suggestion domain.PageCommentSuggestion, sourceLength int) bool {
	return suggestion.StartByte >= 0 && suggestion.EndByte > suggestion.StartByte && suggestion.EndByte <= sourceLength
}

// canApplyInlineSuggestion reports whether the actor may mutate page content after route-level access checks.
func canApplyInlineSuggestion(actor domain.User) bool {
	return actor.CanEditContent()
}

// pageCommentURL returns the stable local fragment for one page discussion item.
func pageCommentURL(slug string, commentID int64) string {
	return "/pages/" + strings.TrimSpace(slug) + "#comment-" + fmt.Sprint(commentID)
}
