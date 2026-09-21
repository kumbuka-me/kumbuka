package pages

import (
	"context"
	"strings"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// personalRepository contains per-user page preference mutations.
type personalRepository interface {
	GetPage(context.Context, string) (domain.Page, error)
	SetFavorite(context.Context, string, int64, bool) error
	SetPageWatch(context.Context, string, int64, string) error
}

// Personal owns favorite and watch mutations for one actor.
type Personal struct {
	repository personalRepository
	access     accessReader
}

// NewPersonal constructs actor-owned page preference commands.
func NewPersonal(repository personalRepository, access accessReader) *Personal {
	return &Personal{repository: repository, access: access}
}

// SetFavoriteFor updates a favorite only when the actor may view an existing page.
func (c *Personal) SetFavoriteFor(ctx context.Context, actor domain.User, slug string, on bool) error {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if err := appaccess.RequireView(ctx, c.access, actor, slug); err != nil {
		return err
	}
	if _, err := c.repository.GetPage(ctx, slug); err != nil {
		return err
	}
	return c.repository.SetFavorite(ctx, slug, actor.ID, on)
}

// SetPageWatchFor updates a watch only when the actor may view an existing page.
func (c *Personal) SetPageWatchFor(ctx context.Context, actor domain.User, slug, scope string) error {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	scope = strings.TrimSpace(scope)
	if !validPageWatchScope(scope) {
		return domain.NewValidationError("scope", "Choose page or subtree notifications.")
	}
	if err := appaccess.RequireView(ctx, c.access, actor, slug); err != nil {
		return err
	}
	if _, err := c.repository.GetPage(ctx, slug); err != nil {
		return err
	}
	return c.repository.SetPageWatch(ctx, slug, actor.ID, scope)
}

// validPageWatchScope reports whether a page-watch scope is empty or one of the supported domain values.
func validPageWatchScope(scope string) bool {
	return scope == "" || scope == domain.PageWatchScopePage || scope == domain.PageWatchScopeSubtree
}
