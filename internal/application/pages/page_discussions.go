package pages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
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

// ErrDiscussionsDisabled indicates that page discussions are globally disabled.
var ErrDiscussionsDisabled = errors.New("page discussions are disabled")

// AddComment adds a discussion comment and emits mention notifications.
func (s *Pages) AddComment(
	ctx context.Context,
	slug string,
	parentID int64,
	anchor, quote, body string,
	actor domain.User,
) (domain.PageComment, error) {
	body = strings.TrimSpace(body)
	if err := s.requireView(ctx, actor, slug); err != nil {
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
	s.notifyMentions(ctx, actor.ID, body, "Mention in "+slug, destination)
	if parentID > 0 {
		if err := s.repository.NotifyCommentReply(ctx, actor.ID, parentID, "Reply in "+slug, destination); err != nil {
			s.logger.ErrorContext(ctx, "comment reply notification failed", "event", "page_side_effect_failed", "error", err)
		}
	}
	s.notifyWatchers(ctx, actor.ID, slug, "New comment: "+slug, "A watched page has a new discussion comment.", destination)
	s.recordAudit(ctx, actor.ID, "comment.created", "page", slug, "Page discussion comment created")

	return comment, nil
}

// AddSuggestion adds an inline Markdown suggestion anchored to uniquely mapped selected page text.
func (s *Pages) AddSuggestion(
	ctx context.Context,
	slug, anchor, body, replacement string,
	actor domain.User,
) (domain.PageComment, error) {
	slug = strings.TrimSpace(slug)
	if err := s.requireView(ctx, actor, slug); err != nil {
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
	if count == 0 || latest.Number <= 0 || latest.Markdown != page.Markdown {
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
		s.notifyMentions(ctx, actor.ID, body, "Mention in "+slug, destination)
	}
	s.notifyWatchers(ctx, actor.ID, slug, "New suggestion: "+slug, "A watched page has a new inline suggestion.", destination)
	s.recordAudit(ctx, actor.ID, "comment.suggested", "page", slug, "Inline Markdown suggestion created")

	return comment, nil
}

// ApplyCommentSuggestion applies one still-current inline suggestion and creates a new page revision.
func (s *Pages) ApplyCommentSuggestion(
	ctx context.Context,
	slug string,
	commentID int64,
	actor domain.User,
) (domain.Page, error) {
	slug = strings.TrimSpace(slug)
	if err := s.requireEdit(ctx, actor, slug); err != nil {
		return domain.Page{}, err
	}
	if slug == "" || commentID <= 0 {
		return domain.Page{}, domain.NewValidationError("suggestion", "Choose a valid inline suggestion.")
	}
	if !canApplyInlineSuggestion(actor) {
		return domain.Page{}, domain.ErrForbidden
	}
	if err := s.requireDiscussions(ctx); err != nil {
		return domain.Page{}, err
	}

	comment, err := s.repository.PageComment(ctx, slug, commentID)
	if err != nil {
		return domain.Page{}, err
	}
	if comment.Suggestion == nil {
		return domain.Page{}, domain.NewValidationError("suggestion", "This comment does not contain an applicable suggestion.")
	}
	if comment.Suggestion.AppliedAt != nil {
		return domain.Page{}, domain.NewValidationError("suggestion", "This suggestion has already been applied.")
	}

	page, err := s.repository.GetPage(ctx, slug)
	if err != nil {
		return domain.Page{}, err
	}
	latest, count, err := s.repository.LatestRevision(ctx, slug)
	if err != nil {
		return domain.Page{}, err
	}
	if count == 0 || latest.Number != comment.Suggestion.RevisionNumber || latest.Markdown != page.Markdown {
		return domain.Page{}, domain.ErrStaleSuggestion
	}

	updatedMarkdown, err := applyInlineSuggestion(page.Markdown, *comment.Suggestion)
	if err != nil {
		return domain.Page{}, err
	}
	usage, render, err := s.derivePageContent(ctx, updatedMarkdown)
	if err != nil {
		return domain.Page{}, err
	}

	message := fmt.Sprintf("Apply inline suggestion from comment #%d", comment.ID)
	updated, err := s.repository.ApplyPageCommentSuggestion(
		ctx,
		slug,
		comment.ID,
		actor.ID,
		updatedMarkdown,
		message,
		md.Links(updatedMarkdown),
		usage,
		render,
	)
	if err != nil {
		return domain.Page{}, err
	}

	s.recordAudit(ctx, actor.ID, "comment.suggestion_applied", "page", updated.Slug, message)
	s.notifyWatchers(ctx, actor.ID, updated.Slug, "Inline suggestion applied: "+updated.Title, message+" and created a new revision.", pageCommentURL(updated.Slug, comment.ID))

	return updated, nil
}

// ResolveComment changes one page-bound discussion's resolution state.
func (s *Pages) ResolveComment(ctx context.Context, slug string, id int64, resolved bool, actor domain.User) error {
	slug = strings.TrimSpace(slug)
	if err := s.requireEdit(ctx, actor, slug); err != nil {
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
func (s *Pages) requireDiscussions(ctx context.Context) error {
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
	if suggestion.StartByte < 0 || suggestion.EndByte <= suggestion.StartByte || suggestion.EndByte > len(markdown) {
		return "", domain.ErrStaleSuggestion
	}
	if markdown[suggestion.StartByte:suggestion.EndByte] != suggestion.Original {
		return "", domain.ErrStaleSuggestion
	}

	return markdown[:suggestion.StartByte] + suggestion.Replacement + markdown[suggestion.EndByte:], nil
}

// canApplyInlineSuggestion reports whether the actor may mutate page content after route-level access checks.
func canApplyInlineSuggestion(actor domain.User) bool {
	return actor.CanEditContent()
}

// pageCommentURL returns the stable local fragment for one page discussion item.
func pageCommentURL(slug string, commentID int64) string {
	return "/pages/" + strings.TrimSpace(slug) + "#comment-" + fmt.Sprint(commentID)
}
