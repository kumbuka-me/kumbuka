package store

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// KnowledgeGraph returns pages and current wiki-link relationships.
func (s *Store) KnowledgeGraph(ctx context.Context, limit int) (domain.KnowledgeGraph, error) {
	if limit <= 0 || limit > 500 {
		limit = 250
	}

	graph := domain.KnowledgeGraph{}
	rows, err := s.pool.Query(ctx, `
SELECT slug,title,status
FROM pages
WHERE deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT $1`, limit)
	if err != nil {
		return graph, err
	}

	allowed := map[string]bool{}

	for rows.Next() {
		var node domain.GraphNode
		if err := rows.Scan(&node.Slug, &node.Title, &node.Status); err != nil {
			rows.Close()
			return graph, err
		}

		graph.Nodes = append(graph.Nodes, node)
		allowed[node.Slug] = true
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return graph, err
	}

	rows.Close()

	rows, err = s.pool.Query(ctx, `
SELECT source.slug,coalesce(target.slug,alias_target.slug,'')
FROM page_links l
JOIN pages source ON source.id=l.source_page_id AND source.deleted_at IS NULL
LEFT JOIN pages target ON target.slug=l.target_slug AND target.deleted_at IS NULL
LEFT JOIN page_aliases a ON a.alias=l.target_slug
LEFT JOIN pages alias_target ON alias_target.id=a.page_id AND alias_target.deleted_at IS NULL
WHERE target.id IS NOT NULL OR alias_target.id IS NOT NULL`)
	if err != nil {
		return graph, err
	}

	defer rows.Close()

	for rows.Next() {
		var edge domain.GraphEdge
		if err := rows.Scan(&edge.Source, &edge.Target); err != nil {
			return graph, err
		}

		if allowed[edge.Source] && allowed[edge.Target] && edge.Source != edge.Target {
			graph.Edges = append(graph.Edges, edge)
		}
	}

	return graph, rows.Err()
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

// DueReviewPages returns pages whose configured review interval has elapsed.
func (s *Store) DueReviewPages(ctx context.Context, limit int) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, `
SELECT slug,title,status,review_interval_days,last_reviewed_at
FROM pages
WHERE deleted_at IS NULL AND review_interval_days>0 AND coalesce(last_reviewed_at,created_at)+(review_interval_days || ' days')::interval <= now()
ORDER BY coalesce(last_reviewed_at,created_at),slug
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.Page

	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.Slug, &page.Title, &page.Status, &page.ReviewIntervalDays, &page.LastReviewedAt); err != nil {
			return nil, err
		}

		pages = append(pages, page)
	}

	return pages, rows.Err()
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
