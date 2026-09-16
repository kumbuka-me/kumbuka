package handler

import (
	"context"
	"errors"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// accessiblePageCatalog limits page-report and include reads to one user's access.
type accessiblePageCatalog struct {
	// catalog stores the catalog value used by accessible page catalog.
	catalog pageReportCatalogService
	// access stores the access value used by accessible page catalog.
	access pageAccessReader
	// user stores the user value used by accessible page catalog.
	user domain.User
}

// GetPage returns the requested page only when the current user may view it.
func (c accessiblePageCatalog) GetPage(ctx context.Context, slug string) (domain.Page, error) {
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
func (c accessiblePageCatalog) Search(ctx context.Context, query string, limit int) ([]domain.Page, error) {
	pages, err := c.catalog.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	return c.access.FilterPages(ctx, c.user, pages)
}

// Backlinks returns only pages visible to the current user that link to slug.
func (c accessiblePageCatalog) Backlinks(ctx context.Context, slug string) ([]domain.Page, error) {
	source, ok := c.catalog.(interface {
		Backlinks(context.Context, string) ([]domain.Page, error)
	})
	if !ok {
		return nil, errors.New("page links are unavailable")
	}
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil || !allowed {
		if err != nil {
			return nil, err
		}
		return nil, domain.ErrNotFound
	}
	pages, err := source.Backlinks(ctx, slug)
	if err != nil {
		return nil, err
	}
	return c.access.FilterPages(ctx, c.user, pages)
}

// PageLinks returns outgoing links without revealing inaccessible target pages.
func (c accessiblePageCatalog) PageLinks(ctx context.Context, slug string) ([]domain.PageLink, error) {
	source, ok := c.catalog.(interface {
		PageLinks(context.Context, string) ([]domain.PageLink, error)
	})
	if !ok {
		return nil, errors.New("page links are unavailable")
	}
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil || !allowed {
		if err != nil {
			return nil, err
		}
		return nil, domain.ErrNotFound
	}
	links, err := source.PageLinks(ctx, slug)
	if err != nil {
		return nil, err
	}
	for index := range links {
		if !links[index].Exists {
			continue
		}
		visible, visibleErr := c.access.CanView(ctx, c.user, links[index].TargetSlug)
		if visibleErr != nil {
			return nil, visibleErr
		}
		if !visible {
			links[index].Exists = false
			links[index].TargetTitle = ""
		}
	}
	return links, nil
}

// Revisions returns revision records only for a page visible to the current user.
func (c accessiblePageCatalog) Revisions(ctx context.Context, slug string) ([]revision.Revision, error) {
	source, ok := c.catalog.(interface {
		Revisions(context.Context, string) ([]revision.Revision, error)
	})
	if !ok {
		return nil, errors.New("page revisions are unavailable")
	}
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil || !allowed {
		if err != nil {
			return nil, err
		}
		return nil, domain.ErrNotFound
	}
	return source.Revisions(ctx, slug)
}

// LatestRevision returns the newest revision and total count for an authorized page.
func (c accessiblePageCatalog) LatestRevision(ctx context.Context, slug string) (revision.Revision, int, error) {
	source, ok := c.catalog.(interface {
		LatestRevision(context.Context, string) (revision.Revision, int, error)
	})
	if !ok {
		return revision.Revision{}, 0, errors.New("page revisions are unavailable")
	}
	allowed, err := c.access.CanView(ctx, c.user, slug)
	if err != nil || !allowed {
		if err != nil {
			return revision.Revision{}, 0, err
		}
		return revision.Revision{}, 0, domain.ErrNotFound
	}
	return source.LatestRevision(ctx, slug)
}

// visibleRecentEdits filters recent edits to pages the user may view.
func visibleRecentEdits(ctx context.Context, access pageAccessReader, user domain.User, edits []domain.RecentEdit) ([]domain.RecentEdit, error) {
	result := make([]domain.RecentEdit, 0, len(edits))

	for _, edit := range edits {
		allowed, err := access.CanView(ctx, user, edit.Slug)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}

		result = append(result, edit)
	}

	return result, nil
}

// visibleKnowledgeGraph removes graph nodes and edges hidden from the user.
func visibleKnowledgeGraph(ctx context.Context, access pageAccessReader, user domain.User, graph domain.KnowledgeGraph) (domain.KnowledgeGraph, error) {
	visible := make(map[string]bool, len(graph.Nodes))
	nodes := make([]domain.GraphNode, 0, len(graph.Nodes))

	for _, node := range graph.Nodes {
		allowed, err := access.CanView(ctx, user, node.Slug)
		if err != nil {
			return domain.KnowledgeGraph{}, err
		}
		if !allowed {
			continue
		}

		visible[node.Slug] = true
		nodes = append(nodes, node)
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

// personalWidgetSource binds personal page lists to the authenticated viewer and access policy.
type personalWidgetSource struct {
	// catalog stores the catalog value used by personal widget source.
	catalog sidebarCatalogService
	// access stores the access value used by personal widget source.
	access pageAccessReader
	// user stores the user value used by personal widget source.
	user domain.User
}

// Favorites returns visible favorites for the current viewer.
func (s personalWidgetSource) Favorites(ctx context.Context, limit int) ([]domain.Page, error) {
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
func (s personalWidgetSource) RecentViewed(ctx context.Context, limit int) ([]domain.Page, error) {
	pages, err := s.catalog.RecentViewed(ctx, s.user.ID, limit)
	if err != nil {
		return nil, err
	}
	return s.access.FilterPages(ctx, s.user, pages)
}

// homeWidgetSource extends personal page lists with dashboard activity and private drafts.
type homeWidgetSource struct {
	// catalog stores the catalog value used by home widget source.
	catalog homeCatalogService
	// drafts stores the drafts value used by home widget source.
	drafts draftListService
	// access stores the access value used by home widget source.
	access pageAccessReader
	// user stores the user value used by home widget source.
	user domain.User
}

// Favorites returns visible favorites for the current viewer.
func (s homeWidgetSource) Favorites(ctx context.Context, limit int) ([]domain.Page, error) {
	return personalWidgetSource{catalog: s.catalog, access: s.access, user: s.user}.Favorites(ctx, limit)
}

// RecentViewed returns visible recently viewed pages for the current viewer.
func (s homeWidgetSource) RecentViewed(ctx context.Context, limit int) ([]domain.Page, error) {
	return personalWidgetSource{catalog: s.catalog, access: s.access, user: s.user}.RecentViewed(ctx, limit)
}

// Recent returns the newest visible pages.
func (s homeWidgetSource) Recent(ctx context.Context, limit int) ([]domain.Page, error) {
	pages, err := s.catalog.ListPages(ctx, limit)
	if err != nil {
		return nil, err
	}
	return s.access.FilterPages(ctx, s.user, pages)
}

// Popular returns the most-viewed visible pages.
func (s homeWidgetSource) Popular(ctx context.Context, limit int) ([]domain.Page, error) {
	pages, err := s.catalog.Popular(ctx, limit)
	if err != nil {
		return nil, err
	}
	return s.access.FilterPages(ctx, s.user, pages)
}

// RecentEdited returns recent edits whose pages remain visible to the current viewer.
func (s homeWidgetSource) RecentEdited(ctx context.Context, limit int) ([]domain.RecentEdit, error) {
	edits, err := s.catalog.RecentEdited(ctx, s.user.ID, limit)
	if err != nil {
		return nil, err
	}
	return visibleRecentEdits(ctx, s.access, s.user, edits)
}

// Drafts returns private draft metadata only for users allowed to edit pages.
func (s homeWidgetSource) Drafts(ctx context.Context, limit int) ([]domain.PageDraft, error) {
	if s.user.Role != "admin" && s.user.Role != "editor" {
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
