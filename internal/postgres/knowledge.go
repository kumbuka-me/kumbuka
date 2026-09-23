package postgres

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// KnowledgeGraph returns pages and current wiki-link relationships.
func (s *Store) KnowledgeGraph(ctx context.Context, limit int) (domain.KnowledgeGraph, error) {
	limit = normalizedKnowledgeGraphLimit(limit)
	nodes, allowed, err := s.knowledgeGraphNodes(ctx, limit)
	if err != nil {
		return domain.KnowledgeGraph{}, err
	}
	edges, err := s.knowledgeGraphEdges(ctx, allowed)
	if err != nil {
		return domain.KnowledgeGraph{}, err
	}
	return domain.KnowledgeGraph{Nodes: nodes, Edges: edges}, nil
}

// normalizedKnowledgeGraphLimit returns the bounded graph node limit.
func normalizedKnowledgeGraphLimit(limit int) int {
	if limit <= 0 || limit > 500 {
		return 250
	}
	return limit
}

// knowledgeGraphNodes loads graph nodes and the set of slugs allowed in edges.
func (s *Store) knowledgeGraphNodes(ctx context.Context, limit int) ([]domain.GraphNode, map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
SELECT slug,title,status
FROM pages
WHERE deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	nodes := make([]domain.GraphNode, 0, limit)
	allowed := make(map[string]bool, limit)
	for rows.Next() {
		var node domain.GraphNode
		if err := rows.Scan(&node.Slug, &node.Title, &node.Status); err != nil {
			return nil, nil, err
		}
		nodes = append(nodes, node)
		allowed[node.Slug] = true
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return nodes, allowed, nil
}

// knowledgeGraphEdges loads existing link edges whose endpoints are both selected nodes.
func (s *Store) knowledgeGraphEdges(ctx context.Context, allowed map[string]bool) ([]domain.GraphEdge, error) {
	rows, err := s.pool.Query(ctx, `
SELECT source.slug,coalesce(target.slug,alias_target.slug,'')
FROM page_links l
JOIN pages source ON source.id=l.source_page_id AND source.deleted_at IS NULL
LEFT JOIN pages target ON target.slug=l.target_slug AND target.deleted_at IS NULL
LEFT JOIN page_aliases a ON a.alias=l.target_slug
LEFT JOIN pages alias_target ON alias_target.id=a.page_id AND alias_target.deleted_at IS NULL
WHERE target.id IS NOT NULL OR alias_target.id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var edges []domain.GraphEdge
	for rows.Next() {
		var edge domain.GraphEdge
		if err := rows.Scan(&edge.Source, &edge.Target); err != nil {
			return nil, err
		}
		if allowed[edge.Source] && allowed[edge.Target] && edge.Source != edge.Target {
			edges = append(edges, edge)
		}
	}
	return edges, rows.Err()
}

// RecentEdited returns pages most recently revised by one user.
func (s *Store) RecentEdited(ctx context.Context, userID int64, limit int) ([]domain.RecentEdit, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.id,p.slug,p.title,coalesce(i.icon,''),p.updated_at,coalesce(r.message,'')
FROM pages p
LEFT JOIN navigation_icons i ON i.path=p.slug
LEFT JOIN LATERAL (SELECT message,created_by FROM page_revisions WHERE page_id=p.id ORDER BY revision_number DESC LIMIT 1) r ON true
WHERE p.deleted_at IS NULL AND r.created_by=$1
ORDER BY p.updated_at DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var items []domain.RecentEdit

	for rows.Next() {
		var item domain.RecentEdit
		if err := rows.Scan(&item.ID, &item.Slug, &item.Title, &item.Icon, &item.UpdatedAt, &item.RevisionMessage); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// MarkPageReviewed records a documentation review timestamp.
func (s *Store) MarkPageReviewed(ctx context.Context, slug string) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE pages
SET last_reviewed_at=now()
WHERE slug=$1 AND deleted_at IS NULL`, slug)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}
