package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// pageDiscussionRepository contains discussion persistence and reply notification operations.
type pageDiscussionRepository interface {
	AddPageComment(context.Context, string, int64, int64, string, string, string) (domain.PageComment, error)
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	NotifyCommentReply(context.Context, int64, int64, string, string) error
	ResolvePageComment(context.Context, int64, bool) error
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
	if body == "" {
		return domain.PageComment{}, domain.NewValidationError("body", "A comment is required.")
	}
	settings, err := s.repository.ApplicationSettings(ctx)
	if err != nil {
		return domain.PageComment{}, err
	}
	if !settings.DiscussionsEnabled {
		return domain.PageComment{}, ErrDiscussionsDisabled
	}

	slug = strings.TrimSpace(slug)
	comment, err := s.repository.AddPageComment(ctx, slug, actor.ID, parentID, anchor, quote, body)
	if err != nil {
		return domain.PageComment{}, err
	}

	destination := "/pages/" + slug + "#comment-" + fmt.Sprint(comment.ID)
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

// ResolveComment changes one discussion's resolution state.
func (s *Pages) ResolveComment(ctx context.Context, id int64, resolved bool) error {
	if id <= 0 {
		return &ValidationError{Fields: []FieldError{{Field: "comment", Message: "Invalid comment."}}}
	}

	settings, err := s.repository.ApplicationSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.DiscussionsEnabled {
		return ErrDiscussionsDisabled
	}

	return s.repository.ResolvePageComment(ctx, id, resolved)
}
