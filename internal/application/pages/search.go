package pages

import (
	"context"
	"slices"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// searchRepository contains page listing, search, and tag discovery reads.
type searchRepository interface {
	ListPagesPage(context.Context, int, int) ([]domain.Page, error)
	SearchPage(context.Context, string, int, int) ([]domain.Page, error)
	TaggedPages(context.Context) ([]domain.Page, error)
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
	return visiblePageWindow(ctx, q.access, actor, limit, q.repository.ListPagesPage)
}

// SearchFor returns only search results visible to the actor.
func (q *Search) SearchFor(ctx context.Context, actor domain.User, query string, limit int) ([]domain.Page, error) {
	return visiblePageWindow(ctx, q.access, actor, limit, func(ctx context.Context, size, offset int) ([]domain.Page, error) {
		return q.repository.SearchPage(ctx, query, size, offset)
	})
}

// TagsFor returns tags attached to pages visible to the actor.
func (q *Search) TagsFor(ctx context.Context, actor domain.User) ([]string, error) {
	pages, err := q.repository.TaggedPages(ctx)
	if err != nil {
		return nil, err
	}

	pages, err = q.access.FilterPages(ctx, actor, pages)
	if err != nil {
		return nil, err
	}

	set := make(map[string]struct{})
	for _, page := range pages {
		for _, tag := range page.Tags {
			set[tag] = struct{}{}
		}
	}

	tags := make([]string, 0, len(set))
	for tag := range set {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	return tags, nil
}
