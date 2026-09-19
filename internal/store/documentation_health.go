package store

import (
	"context"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// DocumentationHealth returns actionable documentation-quality findings.
func (s *Store) DocumentationHealth(ctx context.Context, staleBefore time.Time) (domain.DocumentationHealth, error) {
	var health domain.DocumentationHealth

	rows, err := s.pool.Query(ctx, `
SELECT source.slug,source.title,links.target_slug
FROM page_links links
JOIN pages source ON source.id=links.source_page_id AND source.deleted_at IS NULL
LEFT JOIN pages target ON target.slug=links.target_slug AND target.deleted_at IS NULL
LEFT JOIN page_aliases alias ON alias.alias=links.target_slug
LEFT JOIN pages alias_target ON alias_target.id=alias.page_id AND alias_target.deleted_at IS NULL
WHERE target.id IS NULL AND alias_target.id IS NULL
ORDER BY source.slug,links.target_slug`)
	if err != nil {
		return health, err
	}

	for rows.Next() {
		var item domain.BrokenWikiLink
		if err := rows.Scan(&item.SourceSlug, &item.SourceTitle, &item.TargetSlug); err != nil {
			rows.Close()
			return health, err
		}

		health.BrokenLinks = append(health.BrokenLinks, item)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return health, err
	}

	rows.Close()

	queries := []struct {
		// target receives the pages matching this health category.
		target *[]domain.Page
		// query selects page paths and titles for this category.
		query string
		// args supplies the query parameters in placeholder order.
		args []any
	}{
		{&health.OrphanPages, `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM page_links l JOIN pages source ON source.id=l.source_page_id AND source.deleted_at IS NULL WHERE l.target_slug=p.slug)
ORDER BY p.slug`, nil},
		{&health.UntaggedPages, `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM page_tags pt WHERE pt.page_id=p.id)
ORDER BY p.slug`, nil},
		{&health.UniconedPages, `
SELECT p.slug,p.title
FROM pages p
LEFT JOIN navigation_icons i ON i.path=p.slug
WHERE p.deleted_at IS NULL AND coalesce(i.icon,'')=''
ORDER BY p.slug`, nil},
		{&health.StalePages, `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND p.updated_at<$1
ORDER BY p.updated_at,p.slug`, []any{staleBefore}},
		{&health.ReviewDue, `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND p.review_interval_days>0 AND coalesce(p.last_reviewed_at,p.created_at)+(p.review_interval_days || ' days')::interval <= now()
ORDER BY coalesce(p.last_reviewed_at,p.created_at),p.slug`, nil},
		{&health.DraftPages, `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND p.status='draft'
ORDER BY p.slug`, nil},
		{&health.Deprecated, `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND p.status='deprecated'
ORDER BY p.slug`, nil},
	}

	for _, item := range queries {
		rows, err := s.pool.Query(ctx, item.query, item.args...)
		if err != nil {
			return health, err
		}

		for rows.Next() {
			var page domain.Page
			if err := rows.Scan(&page.Slug, &page.Title); err != nil {
				rows.Close()
				return health, err
			}

			*item.target = append(*item.target, page)
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return health, err
		}

		rows.Close()
	}

	return health, nil
}
