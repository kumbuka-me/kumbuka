package service

import (
	"context"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// catalogRepository contains page retrieval and discovery operations.
type catalogRepository interface {
	GetPage(context.Context, string) (domain.Page, error)
	SavePageRender(context.Context, int64, time.Time, domain.PageRender) error
	ListPages(context.Context, int) ([]domain.Page, error)
	Search(context.Context, string, int) ([]domain.Page, error)
	RecentViewed(context.Context, int64, int) ([]domain.Page, error)
	RecentEdited(context.Context, int64, int) ([]domain.RecentEdit, error)
	Popular(context.Context, int) ([]domain.Page, error)
	RecordView(context.Context, string, int64) error
	SetFavorite(context.Context, string, int64, bool) error
	IsFavorite(context.Context, string, int64) (bool, error)
	Favorites(context.Context, int64) ([]domain.Page, error)
	Backlinks(context.Context, string) ([]domain.Page, error)
	PageLinks(context.Context, string) ([]domain.PageLink, error)
	ResolvePageAlias(context.Context, string) (string, error)
	LatestRevision(context.Context, string) (record revision.Revision, count int, err error)
	Revisions(context.Context, string) ([]revision.Revision, error)
	Tags(context.Context) ([]string, error)
	PageAliases(context.Context) (map[string]string, error)
	PageInventory(context.Context) ([]domain.Page, error)
	PageComments(context.Context, string) ([]domain.PageComment, error)
	PageSlugByID(context.Context, int64) (string, error)
	PageWatch(context.Context, string, int64) (domain.PageWatch, error)
	SetPageWatch(context.Context, string, int64, string) error
}

// Catalog exposes page retrieval and discovery use cases.
type Catalog struct{ repository catalogRepository }

// NewCatalog constructs the page catalog service.
func NewCatalog(repository catalogRepository) *Catalog { return &Catalog{repository: repository} }

// GetPage returns an active page by slug.
func (s *Catalog) GetPage(ctx context.Context, slug string) (domain.Page, error) {
	return s.repository.GetPage(ctx, slug)
}

// SavePageRender refreshes a reusable render artifact for an unchanged page.
func (s *Catalog) SavePageRender(ctx context.Context, pageID int64, updatedAt time.Time, render domain.PageRender) error {
	return s.repository.SavePageRender(ctx, pageID, updatedAt, render)
}

// ListPages returns recently updated active pages up to a limit.
func (s *Catalog) ListPages(ctx context.Context, limit int) ([]domain.Page, error) {
	return s.repository.ListPages(ctx, limit)
}

// Search returns pages matching a structured full-text query.
func (s *Catalog) Search(ctx context.Context, query string, limit int) ([]domain.Page, error) {
	return s.repository.Search(ctx, query, limit)
}

// RecentViewed returns the pages most recently viewed by a user.
func (s *Catalog) RecentViewed(ctx context.Context, userID int64, limit int) ([]domain.Page, error) {
	return s.repository.RecentViewed(ctx, userID, limit)
}

// RecentEdited returns the pages most recently edited by a user.
func (s *Catalog) RecentEdited(ctx context.Context, userID int64, limit int) ([]domain.RecentEdit, error) {
	return s.repository.RecentEdited(ctx, userID, limit)
}

// Popular returns the most viewed active pages.
func (s *Catalog) Popular(ctx context.Context, limit int) ([]domain.Page, error) {
	return s.repository.Popular(ctx, limit)
}

// RecordView records that a user viewed a page.
func (s *Catalog) RecordView(ctx context.Context, slug string, userID int64) error {
	return s.repository.RecordView(ctx, slug, userID)
}

// SetFavorite adds or removes a page from a user's favorites.
func (s *Catalog) SetFavorite(ctx context.Context, slug string, userID int64, on bool) error {
	return s.repository.SetFavorite(ctx, slug, userID, on)
}

// IsFavorite reports whether a page is a favorite of the user.
func (s *Catalog) IsFavorite(ctx context.Context, slug string, userID int64) (bool, error) {
	return s.repository.IsFavorite(ctx, slug, userID)
}

// Favorites returns a user's favorite pages.
func (s *Catalog) Favorites(ctx context.Context, userID int64) ([]domain.Page, error) {
	return s.repository.Favorites(ctx, userID)
}

// Backlinks returns active pages linking to the supplied slug.
func (s *Catalog) Backlinks(ctx context.Context, slug string) ([]domain.Page, error) {
	return s.repository.Backlinks(ctx, slug)
}

// PageLinks returns outgoing and incoming link metadata for a page.
func (s *Catalog) PageLinks(ctx context.Context, slug string) ([]domain.PageLink, error) {
	return s.repository.PageLinks(ctx, slug)
}

// ResolvePageAlias resolves a historical slug to its active page slug.
func (s *Catalog) ResolvePageAlias(ctx context.Context, alias string) (string, error) {
	return s.repository.ResolvePageAlias(ctx, alias)
}

// LatestRevision returns a page's newest revision and total revision count.
func (s *Catalog) LatestRevision(ctx context.Context, slug string) (record revision.Revision, count int, err error) {
	return s.repository.LatestRevision(ctx, slug)
}

// Revisions returns a page's revision history.
func (s *Catalog) Revisions(ctx context.Context, slug string) ([]revision.Revision, error) {
	return s.repository.Revisions(ctx, slug)
}

// Tags returns all known page tags.
func (s *Catalog) Tags(ctx context.Context) ([]string, error) {
	return s.repository.Tags(ctx)
}

// PageAliases returns historical page slugs keyed by their active targets.
func (s *Catalog) PageAliases(ctx context.Context) (map[string]string, error) {
	return s.repository.PageAliases(ctx)
}

// PageInventory returns pages with lifecycle metadata for administration.
func (s *Catalog) PageInventory(ctx context.Context) ([]domain.Page, error) {
	return s.repository.PageInventory(ctx)
}

// PageComments returns discussion comments attached to a page.
func (s *Catalog) PageComments(ctx context.Context, slug string) ([]domain.PageComment, error) {
	return s.repository.PageComments(ctx, slug)
}

// PageWatch returns the current user's exact-path watch.
func (s *Catalog) PageWatch(ctx context.Context, slug string, userID int64) (domain.PageWatch, error) {
	return s.repository.PageWatch(ctx, slug, userID)
}

// SetPageWatch creates, changes, or removes one user's exact-path watch.
func (s *Catalog) SetPageWatch(ctx context.Context, slug string, userID int64, scope string) error {
	return s.repository.SetPageWatch(ctx, slug, userID, scope)
}
