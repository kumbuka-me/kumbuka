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
	// repository loads aliases and page inventory metadata.
	repository directoryRepository
	// access filters inventory rows for an authenticated actor.
	access accessReader
}

// NewDirectory constructs page directory queries.
func NewDirectory(repository directoryRepository, access accessReader) *Directory {
	return &Directory{repository: repository, access: access}
}

// PageAliases returns historical page slugs keyed by their active targets.
func (q *Directory) PageAliases(ctx context.Context) (map[string]string, error) {
	return q.repository.PageAliases(ctx)
}

// PageAliasesFor returns only aliases whose active target is visible to the actor.
func (q *Directory) PageAliasesFor(ctx context.Context, actor domain.User) (map[string]string, error) {
	aliases, err := q.repository.PageAliases(ctx)
	if err != nil {
		return nil, err
	}

	targets := aliasTargets(aliases)
	visibleTargets, err := q.access.FilterPages(ctx, actor, targets)
	if err != nil {
		return nil, err
	}

	visible := make(map[string]struct{}, len(visibleTargets))
	for _, page := range visibleTargets {
		visible[page.Slug] = struct{}{}
	}

	result := make(map[string]string, len(aliases))
	for alias, target := range aliases {
		if _, ok := visible[target]; ok {
			result[alias] = target
		}
	}

	return result, nil
}

// aliasTargets converts alias targets into the page projection required by access filtering.
func aliasTargets(aliases map[string]string) []domain.Page {
	seen := make(map[string]struct{}, len(aliases))
	targets := make([]domain.Page, 0, len(aliases))
	for _, target := range aliases {
		if _, ok := seen[target]; ok {
			continue
		}

		seen[target] = struct{}{}
		targets = append(targets, domain.Page{Slug: target})
	}

	return targets
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
