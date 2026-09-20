package pages

import (
	"context"
	"errors"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// lookupRepository contains persistence required to resolve one page.
type lookupRepository interface {
	GetPage(context.Context, string) (domain.Page, error)
	ResolvePageAlias(context.Context, string) (string, error)
	PageSlugByID(context.Context, int64) (string, error)
}

// Lookup owns direct page lookup and alias resolution.
type Lookup struct {
	repository lookupRepository
	access     accessReader
}

// NewLookup constructs direct page lookup use cases.
func NewLookup(repository lookupRepository, access accessReader) *Lookup {
	return &Lookup{repository: repository, access: access}
}

// GetPage returns an active page by slug without actor filtering.
// It is intended for already-authorized administrator and export workflows.
func (q *Lookup) GetPage(ctx context.Context, slug string) (domain.Page, error) {
	return q.repository.GetPage(ctx, slug)
}

// GetPageFor returns one page only when the actor may view it.
func (q *Lookup) GetPageFor(ctx context.Context, actor domain.User, slug string) (domain.Page, error) {
	if err := appaccess.RequireView(ctx, q.access, actor, slug); err != nil {
		return domain.Page{}, err
	}
	return q.repository.GetPage(ctx, slug)
}

// GetPageForEdit returns one page only when the actor may edit its path.
func (q *Lookup) GetPageForEdit(ctx context.Context, actor domain.User, slug string) (domain.Page, error) {
	if err := appaccess.RequireEdit(ctx, q.access, actor, slug); err != nil {
		return domain.Page{}, err
	}
	return q.repository.GetPage(ctx, slug)
}

// GetPageOrAliasFor returns a visible page and reports the resolved alias target when applicable.
func (q *Lookup) GetPageOrAliasFor(ctx context.Context, actor domain.User, slug string) (domain.Page, string, error) {
	page, err := q.GetPageFor(ctx, actor, slug)
	if err == nil {
		return page, "", nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Page{}, "", err
	}

	target, err := q.repository.ResolvePageAlias(ctx, slug)
	if err != nil {
		return domain.Page{}, "", err
	}
	page, err = q.GetPageFor(ctx, actor, target)
	if err != nil {
		return domain.Page{}, "", err
	}
	return page, target, nil
}

// PageSlugByID resolves the current page path for a stable page identifier.
func (q *Lookup) PageSlugByID(ctx context.Context, id int64) (string, error) {
	return q.repository.PageSlugByID(ctx, id)
}
