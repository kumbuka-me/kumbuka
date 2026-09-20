package pages

import (
	"context"
	"slices"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// AccessibleCatalog limits page-report and include reads to one user's access.
type AccessibleCatalog struct {
	// catalog stores the catalog value used by accessible page catalog.
	catalog reportReader
	// access stores the access value used by accessible page catalog.
	access accessReader
	// user stores the user value used by accessible page catalog.
	user domain.User
}

// GetPage returns the requested page only when the current user may view it.
func (c AccessibleCatalog) GetPage(ctx context.Context, slug string) (domain.Page, error) {
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil {
		return domain.Page{}, err
	}
	if !allowed {
		return domain.Page{}, domain.ErrNotFound
	}
	return c.catalog.GetPage(ctx, slug)
}

// Search returns only report pages visible to the current user.
func (c AccessibleCatalog) Search(ctx context.Context, query string, limit int) ([]domain.Page, error) {
	pages, err := c.catalog.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return c.access.FilterPages(ctx, c.user, pages)
}

// Backlinks returns only pages visible to the current user that link to slug.
func (c AccessibleCatalog) Backlinks(ctx context.Context, slug string) ([]domain.Page, error) {
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil || !allowed {
		if err != nil {
			return nil, err
		}
		return nil, domain.ErrNotFound
	}
	pages, err := c.catalog.Backlinks(ctx, slug)
	if err != nil {
		return nil, err
	}
	return c.access.FilterPages(ctx, c.user, pages)
}

// PageLinks returns outgoing links without revealing inaccessible target pages.
func (c AccessibleCatalog) PageLinks(ctx context.Context, slug string) ([]domain.PageLink, error) {
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil || !allowed {
		if err != nil {
			return nil, err
		}
		return nil, domain.ErrNotFound
	}
	links, err := c.catalog.PageLinks(ctx, slug)
	if err != nil {
		return nil, err
	}
	paths := make([]domain.Page, 0, len(links))
	for _, link := range links {
		if link.Exists {
			paths = append(paths, domain.Page{Slug: link.TargetSlug})
		}
	}
	visible, err := visiblePaths(ctx, c.access, c.user, paths)
	if err != nil {
		return nil, err
	}
	// Own the result slice before redacting inaccessible target metadata.
	links = slices.Clone(links)
	for index := range links {
		if links[index].Exists && !visible[links[index].TargetSlug] {
			links[index].Exists = false
			links[index].TargetTitle = ""
		}
	}

	return links, nil
}

// Revisions returns revision records only for a page visible to the current user.
func (c AccessibleCatalog) Revisions(ctx context.Context, slug string) ([]revision.Revision, error) {
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil || !allowed {
		if err != nil {
			return nil, err
		}
		return nil, domain.ErrNotFound
	}
	return c.catalog.Revisions(ctx, slug)
}

// LatestRevision returns the newest revision and total count for an authorized page.
func (c AccessibleCatalog) LatestRevision(ctx context.Context, slug string) (revision.Revision, int, error) {
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil || !allowed {
		if err != nil {
			return revision.Revision{}, 0, err
		}
		return revision.Revision{}, 0, domain.ErrNotFound
	}
	return c.catalog.LatestRevision(ctx, slug)
}

// visibleRecentEdits filters recent edits to pages the user may view.
func visibleRecentEdits(ctx context.Context, access accessReader, user domain.User, edits []domain.RecentEdit) ([]domain.RecentEdit, error) {
	paths := make([]domain.Page, len(edits))
	for i, edit := range edits {
		paths[i].Slug = edit.Slug
	}
	visible, err := visiblePaths(ctx, access, user, paths)
	if err != nil {
		return nil, err
	}
	result := make([]domain.RecentEdit, 0, len(edits))
	for _, edit := range edits {
		if visible[edit.Slug] {
			result = append(result, edit)
		}
	}

	return result, nil
}

// VisibleKnowledgeGraph removes graph nodes and edges hidden from the user.
func VisibleKnowledgeGraph(ctx context.Context, access accessReader, user domain.User, graph domain.KnowledgeGraph) (domain.KnowledgeGraph, error) {
	paths := make([]domain.Page, len(graph.Nodes))
	for i, node := range graph.Nodes {
		paths[i].Slug = node.Slug
	}
	visible, err := visiblePaths(ctx, access, user, paths)
	if err != nil {
		return domain.KnowledgeGraph{}, err
	}
	nodes := make([]domain.GraphNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if visible[node.Slug] {
			nodes = append(nodes, node)
		}
	}

	edges := make([]domain.GraphEdge, 0, len(graph.Edges))

	for _, edge := range graph.Edges {
		if !visible[edge.Source] || !visible[edge.Target] {
			continue
		}

		edges = append(edges, edge)
	}

	graph.Nodes = nodes
	graph.Edges = edges

	return graph, nil
}

// HomeLists extends personal page lists with dashboard activity and private drafts.
type HomeLists struct {
	// catalog stores the catalog value used by home widget source.
	catalog homeReader
	// drafts stores the drafts value used by home widget source.
	drafts draftReader
	// access stores the access value used by home widget source.
	access accessReader
	// user stores the user value used by home widget source.
	user domain.User
}

// Favorites returns visible favorites for the current viewer.
func (s HomeLists) Favorites(ctx context.Context, limit int) ([]domain.Page, error) {
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
func (s HomeLists) RecentViewed(ctx context.Context, limit int) ([]domain.Page, error) {
	pages, err := s.catalog.RecentViewed(ctx, s.user.ID, limit)
	if err != nil {
		return nil, err
	}
	return s.access.FilterPages(ctx, s.user, pages)
}

// Recent returns the newest visible pages.
func (s HomeLists) Recent(ctx context.Context, limit int) ([]domain.Page, error) {
	pages, err := s.catalog.ListPages(ctx, limit)
	if err != nil {
		return nil, err
	}
	return s.access.FilterPages(ctx, s.user, pages)
}

// Popular returns the most-viewed visible pages.
func (s HomeLists) Popular(ctx context.Context, limit int) ([]domain.Page, error) {
	pages, err := s.catalog.Popular(ctx, limit)
	if err != nil {
		return nil, err
	}
	return s.access.FilterPages(ctx, s.user, pages)
}

// RecentEdited returns recent edits whose pages remain visible to the current viewer.
func (s HomeLists) RecentEdited(ctx context.Context, limit int) ([]domain.RecentEdit, error) {
	edits, err := s.catalog.RecentEdited(ctx, s.user.ID, limit)
	if err != nil {
		return nil, err
	}
	return visibleRecentEdits(ctx, s.access, s.user, edits)
}

// Drafts returns private draft metadata only for users allowed to edit pages.
func (s HomeLists) Drafts(ctx context.Context, limit int) ([]domain.PageDraft, error) {
	if !s.user.CanEditContent() {
		return nil, nil
	}
	return s.drafts.List(ctx, s.user.ID, limit)
}

// limitPages truncates a page list to the requested maximum size.
func limitPages(pages []domain.Page, limit int) []domain.Page {
	if limit <= 0 || len(pages) <= limit {
		return pages
	}
	return pages[:limit]
}

// reportReader supplies the generic page search and include capabilities.
type reportReader interface {
	GetPage(context.Context, string) (domain.Page, error)
	Search(context.Context, string, int) ([]domain.Page, error)
	Backlinks(context.Context, string) ([]domain.Page, error)
	PageLinks(context.Context, string) ([]domain.PageLink, error)
	LatestRevision(context.Context, string) (revision.Revision, int, error)
	Revisions(context.Context, string) ([]revision.Revision, error)
}

// accessReader evaluates one resource or filters a collection for an actor.
type accessReader interface {
	CanView(context.Context, domain.User, string) (bool, error)
	CanEdit(context.Context, domain.User, string) (bool, error)
	FilterPages(context.Context, domain.User, []domain.Page) ([]domain.Page, error)
}

type homeReader interface {
	Favorites(context.Context, int64) ([]domain.Page, error)
	RecentViewed(context.Context, int64, int) ([]domain.Page, error)
	ListPages(context.Context, int) ([]domain.Page, error)
	Popular(context.Context, int) ([]domain.Page, error)
	RecentEdited(context.Context, int64, int) ([]domain.RecentEdit, error)
}

type draftReader interface {
	List(context.Context, int64, int) ([]domain.PageDraft, error)
}

// NewAccessibleCatalog binds generic plugin capabilities to one authorized actor.
func NewAccessibleCatalog(catalog reportReader, access accessReader, actor domain.User) AccessibleCatalog {
	return AccessibleCatalog{catalog: catalog, access: access, user: actor}
}

// visiblePaths performs one bulk access query and preserves the caller's collections.
func visiblePaths(ctx context.Context, access accessReader, user domain.User, paths []domain.Page) (map[string]bool, error) {
	pages, err := access.FilterPages(ctx, user, paths)
	if err != nil {
		return nil, err
	}
	visible := make(map[string]bool, len(pages))
	for _, page := range pages {
		visible[page.Slug] = true
	}
	return visible, nil
}

// HomeQuery binds dashboard page-list capabilities to one actor without exposing access policy wiring to HTTP.
type HomeQuery struct {
	// catalog supplies dashboard page lists.
	catalog homeReader
	// drafts supplies private draft lists.
	drafts draftReader
	// access filters all page collections for the actor.
	access accessReader
}

// NewHomeQuery constructs the dashboard page-list query.
func NewHomeQuery(catalog homeReader, drafts draftReader, access accessReader) *HomeQuery {
	return &HomeQuery{catalog: catalog, drafts: drafts, access: access}
}

// Lists returns dashboard capabilities scoped to one actor.
func (q *HomeQuery) Lists(actor domain.User) HomeLists {
	return HomeLists{catalog: q.catalog, drafts: q.drafts, access: q.access, user: actor}
}
