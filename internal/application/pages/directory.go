package pages

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// directoryRepository contains page metadata used by administration and editor catalogs.
type directoryRepository interface {
	PageAliases(context.Context) (map[string]string, error)
	PageInventory(context.Context) ([]domain.Page, error)
}

// Directory owns page metadata and inventory queries.
type Directory struct {
	repository directoryRepository
	access     accessReader
}

// NewDirectory constructs page directory queries.
func NewDirectory(repository directoryRepository, access accessReader) *Directory {
	return &Directory{repository: repository, access: access}
}

// PageAliases returns historical page slugs keyed by their active targets.
func (q *Directory) PageAliases(ctx context.Context) (map[string]string, error) {
	return q.repository.PageAliases(ctx)
}

// PageInventory returns pages with lifecycle metadata for administration.
func (q *Directory) PageInventory(ctx context.Context) ([]domain.Page, error) {
	return q.repository.PageInventory(ctx)
}

// PageInventoryFor returns only inventory rows visible to the actor.
func (q *Directory) PageInventoryFor(ctx context.Context, actor domain.User) ([]domain.Page, error) {
	pages, err := q.repository.PageInventory(ctx)
	if err != nil {
		return nil, err
	}
	return q.access.FilterPages(ctx, actor, pages)
}
