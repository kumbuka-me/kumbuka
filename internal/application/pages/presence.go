package pages

import (
	"context"
	"strings"
	"time"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const pageEditorPresenceTTL = 90 * time.Second

// pagePresenceRepository persists short-lived collaborative editor presence.
type pagePresenceRepository interface {
	TouchPageEditor(context.Context, string, int64) error
	LeavePageEditor(context.Context, string, int64) error
	PageEditors(context.Context, string, int64, time.Duration) ([]domain.PageEditorPresence, error)
}

// Presence owns short-lived collaborative editor presence.
type Presence struct {
	repository    pagePresenceRepository
	authorization pageAuthorization
}

// NewPresence constructs page editor presence use cases.
func NewPresence(repository pagePresenceRepository, access appaccess.Policy) *Presence {
	return &Presence{repository: repository, authorization: pageAuthorization{policy: access}}
}

// PageEditors returns other users with a recent editor heartbeat for one page.
func (s *Presence) PageEditors(ctx context.Context, slug string, actor domain.User) ([]domain.PageEditorPresence, error) {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if err := s.authorization.requireView(ctx, actor, slug); err != nil {
		return nil, err
	}
	if slug == "" {
		return nil, domain.ErrNotFound
	}

	return s.repository.PageEditors(ctx, slug, actor.ID, pageEditorPresenceTTL)
}

// TouchPageEditor refreshes one authenticated user's editor presence.
func (s *Presence) TouchPageEditor(ctx context.Context, slug string, actor domain.User) error {
	if actor.ID <= 0 {
		return domain.ErrForbidden
	}

	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return domain.ErrNotFound
	}
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return err
	}

	return s.repository.TouchPageEditor(ctx, slug, actor.ID)
}

// LeavePageEditor clears one authenticated user's editor presence.
func (s *Presence) LeavePageEditor(ctx context.Context, slug string, actor domain.User) error {
	if actor.ID <= 0 {
		return domain.ErrForbidden
	}

	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return nil
	}
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return err
	}

	return s.repository.LeavePageEditor(ctx, slug, actor.ID)
}
