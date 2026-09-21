package pages

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// searchRepository contains page listing, search, and tag discovery reads.
type searchRepository interface {
	ListPages(context.Context, int) ([]domain.Page, error)
	Search(context.Context, string, int) ([]domain.Page, error)
	Tags(context.Context) ([]string, error)
}

// Search owns actor-filtered page discovery queries.
type Search struct {
	// repository loads page lists, search matches, and known tags.
	repository searchRepository
	// access filters page results for an authenticated actor.
	access accessReader
}

// NewSearch constructs page discovery queries.
func NewSearch(repository searchRepository, access accessReader) *Search {
	return &Search{repository: repository, access: access}
}

// ListPagesFor returns only pages visible to the actor.
func (q *Search) ListPagesFor(ctx context.Context, actor domain.User, limit int) ([]domain.Page, error) {
	pages, err := q.repository.ListPages(ctx, limit)
	if err != nil {
		return nil, err
	}
	return q.access.FilterPages(ctx, actor, pages)
}

// SearchFor returns only search results visible to the actor.
func (q *Search) SearchFor(ctx context.Context, actor domain.User, query string, limit int) ([]domain.Page, error) {
	pages, err := q.repository.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return q.access.FilterPages(ctx, actor, pages)
}

// Tags returns all known page tags.
func (q *Search) Tags(ctx context.Context) ([]string, error) {
	return q.repository.Tags(ctx)
}
