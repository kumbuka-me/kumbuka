package service

import (
	"context"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// knowledgeRepository contains knowledge graph and saved-search operations.
type knowledgeRepository interface {
	auditRepository
	KnowledgeGraph(context.Context, int) (domain.KnowledgeGraph, error)
	SavedSearches(context.Context, int64) ([]domain.SavedSearch, error)
	SaveSavedSearch(context.Context, int64, int64, string, string, bool) error
	DeleteSavedSearch(context.Context, int64, int64) error
}

// Knowledge exposes knowledge graph and saved-search use cases.
type Knowledge struct{ repository knowledgeRepository }

// NewKnowledge constructs the knowledge tools service.
func NewKnowledge(repository knowledgeRepository) *Knowledge {
	return &Knowledge{repository: repository}
}

// KnowledgeGraph returns page nodes and links for graph rendering.
func (s *Knowledge) KnowledgeGraph(ctx context.Context, limit int) (domain.KnowledgeGraph, error) {
	return s.repository.KnowledgeGraph(ctx, limit)
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
