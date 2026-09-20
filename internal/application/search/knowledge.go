package search

import (
	"context"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/application/audit"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// knowledgeRepository contains knowledge graph and saved-search operations.
type knowledgeRepository interface {
	audit.Repository
	KnowledgeGraph(context.Context, int) (domain.KnowledgeGraph, error)
	SavedSearches(context.Context, int64) ([]domain.SavedSearch, error)
	SaveSavedSearch(context.Context, int64, int64, string, string, bool) error
	DeleteSavedSearch(context.Context, int64, int64) error
}

// pageFilter removes graph nodes the actor may not view.
type pageFilter interface {
	FilterPages(context.Context, domain.User, []domain.Page) ([]domain.Page, error)
}

// Knowledge exposes knowledge graph and saved-search use cases.
type Knowledge struct {
	// repository provides the persistence operations required by knowledge.
	repository knowledgeRepository
	// access filters graph nodes and edges for the current actor.
	access pageFilter
}

// NewKnowledge constructs the knowledge tools service.
func NewKnowledge(repository knowledgeRepository, access pageFilter) *Knowledge {
	return &Knowledge{repository: repository, access: access}
}

// KnowledgeGraph returns page nodes and links for graph rendering.
func (s *Knowledge) KnowledgeGraph(ctx context.Context, limit int) (domain.KnowledgeGraph, error) {
	return s.repository.KnowledgeGraph(ctx, limit)
}

// KnowledgeGraphFor returns only graph nodes and edges visible to the actor.
func (s *Knowledge) KnowledgeGraphFor(ctx context.Context, actor domain.User, limit int) (domain.KnowledgeGraph, error) {
	graph, err := s.KnowledgeGraph(ctx, limit)
	if err != nil {
		return domain.KnowledgeGraph{}, err
	}
	pages := make([]domain.Page, len(graph.Nodes))
	for i, node := range graph.Nodes {
		pages[i].Slug = node.Slug
	}
	visiblePages, err := s.access.FilterPages(ctx, actor, pages)
	if err != nil {
		return domain.KnowledgeGraph{}, err
	}
	visible := make(map[string]bool, len(visiblePages))
	for _, page := range visiblePages {
		visible[page.Slug] = true
	}
	nodes := make([]domain.GraphNode, 0, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if visible[node.Slug] {
			nodes = append(nodes, node)
		}
	}
	edges := make([]domain.GraphEdge, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		if visible[edge.Source] && visible[edge.Target] {
			edges = append(edges, edge)
		}
	}
	graph.Nodes = nodes
	graph.Edges = edges
	return graph, nil
}

// SavedSearches returns the searches saved by a user.
func (s *Knowledge) SavedSearches(ctx context.Context, userID int64) ([]domain.SavedSearch, error) {
	return s.repository.SavedSearches(ctx, userID)
}

// SaveSavedSearch creates or updates a user's saved search.
func (s *Knowledge) SaveSavedSearch(
	ctx context.Context,
	userID, id int64,
	name, query string,
	pinned bool,
) error {
	name = strings.TrimSpace(name)
	query = strings.TrimSpace(query)
	if name == "" {
		return domain.NewValidationError("name", "A saved search name is required.")
	}
	if query == "" {
		return domain.NewValidationError("query", "A search query is required.")
	}
	return s.repository.SaveSavedSearch(ctx, userID, id, name, query, pinned)
}

// DeleteSavedSearch removes a saved search owned by a user.
func (s *Knowledge) DeleteSavedSearch(ctx context.Context, userID, id int64) error {
	return s.repository.DeleteSavedSearch(ctx, userID, id)
}
