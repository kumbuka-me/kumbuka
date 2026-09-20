package viewer

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PersonalLists binds personal page lists to the authenticated viewer and access policy.
type PersonalLists struct {
	// catalog loads personal page lists.
	catalog sidebarCatalogReader
	// access filters those lists for the current viewer.
	access accessReader
	// user is the authenticated viewer.
	user domain.User
}

// Favorites returns visible favorites for the current viewer.
func (s PersonalLists) Favorites(ctx context.Context, limit int) ([]domain.Page, error) {
	pages, err := s.catalog.Favorites(ctx, s.user.ID)
	if err != nil {
		return nil, err
	}
	pages, err = s.access.FilterPages(ctx, s.user, pages)
	if err != nil {
		return nil, err
	}
	return limitPages(pages, limit), nil
}

// RecentViewed returns visible recently viewed pages for the current viewer.
func (s PersonalLists) RecentViewed(ctx context.Context, limit int) ([]domain.Page, error) {
	pages, err := s.catalog.RecentViewed(ctx, s.user.ID, limit)
	if err != nil {
		return nil, err
	}
	return s.access.FilterPages(ctx, s.user, pages)
}

// limitPages truncates a page list to the requested maximum size.
func limitPages(pages []domain.Page, limit int) []domain.Page {
	if limit <= 0 || len(pages) <= limit {
		return pages
	}
	return pages[:limit]
}
