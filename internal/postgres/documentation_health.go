package postgres

import (
	"context"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const brokenWikiLinksQuery = `
SELECT source.slug,source.title,links.target_slug
FROM page_links links
JOIN pages source ON source.id=links.source_page_id AND source.deleted_at IS NULL
LEFT JOIN pages target ON target.slug=links.target_slug AND target.deleted_at IS NULL
LEFT JOIN page_aliases alias ON alias.alias=links.target_slug
LEFT JOIN pages alias_target ON alias_target.id=alias.page_id AND alias_target.deleted_at IS NULL
WHERE target.id IS NULL AND alias_target.id IS NULL
ORDER BY source.slug,links.target_slug`

const orphanPagesQuery = `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND NOT EXISTS (
  SELECT 1
  FROM page_links l
  JOIN pages source ON source.id=l.source_page_id AND source.deleted_at IS NULL
  LEFT JOIN page_aliases alias ON alias.alias=l.target_slug
  WHERE l.target_slug=p.slug OR alias.page_id=p.id
)
ORDER BY p.slug`

const untaggedPagesQuery = `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM page_tags pt WHERE pt.page_id=p.id)
ORDER BY p.slug`

const uniconedPagesQuery = `
SELECT p.slug,p.title
FROM pages p
LEFT JOIN navigation_icons i ON i.path=p.slug
WHERE p.deleted_at IS NULL AND coalesce(i.icon,'')=''
ORDER BY p.slug`

const stalePagesQuery = `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND p.updated_at<$1
ORDER BY p.updated_at,p.slug`

const reviewDuePagesQuery = `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND p.review_interval_days>0 AND coalesce(p.last_reviewed_at,p.created_at)+(p.review_interval_days || ' days')::interval <= now()
ORDER BY coalesce(p.last_reviewed_at,p.created_at),p.slug`

const draftPagesQuery = `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND p.status='draft'
ORDER BY p.slug`

const deprecatedPagesQuery = `
SELECT p.slug,p.title
FROM pages p
WHERE p.deleted_at IS NULL AND p.status='deprecated'
ORDER BY p.slug`

// documentationHealthPageQuery describes one page-list category in the documentation-health report.
type documentationHealthPageQuery struct {
	// target receives the pages returned by statement.
	target *[]domain.Page
	// statement selects page slugs and titles for the category.
	statement string
	// args contains positional SQL arguments for statement.
	args []any
}

// DocumentationHealth returns actionable documentation-quality findings.
func (s *Store) DocumentationHealth(ctx context.Context, staleBefore time.Time) (domain.DocumentationHealth, error) {
	var health domain.DocumentationHealth

	brokenLinks, err := s.brokenWikiLinks(ctx)
	if err != nil {
		return health, err
	}
	health.BrokenLinks = brokenLinks

	queries := []documentationHealthPageQuery{
		{target: &health.OrphanPages, statement: orphanPagesQuery},
		{target: &health.UntaggedPages, statement: untaggedPagesQuery},
		{target: &health.UniconedPages, statement: uniconedPagesQuery},
		{target: &health.StalePages, statement: stalePagesQuery, args: []any{staleBefore}},
		{target: &health.ReviewDue, statement: reviewDuePagesQuery},
		{target: &health.DraftPages, statement: draftPagesQuery},
		{target: &health.Deprecated, statement: deprecatedPagesQuery},
	}

	for _, query := range queries {
		pages, err := s.documentationHealthPages(ctx, query.statement, query.args...)
		if err != nil {
			return health, err
		}

		*query.target = pages
	}

	return health, nil
}

// brokenWikiLinks returns links that resolve to neither an active page nor one of its aliases.
func (s *Store) brokenWikiLinks(ctx context.Context) ([]domain.BrokenWikiLink, error) {
	rows, err := s.pool.Query(ctx, brokenWikiLinksQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []domain.BrokenWikiLink
	for rows.Next() {
		var link domain.BrokenWikiLink
		if err := rows.Scan(&link.SourceSlug, &link.SourceTitle, &link.TargetSlug); err != nil {
			return nil, err
		}

		links = append(links, link)
	}

	return links, rows.Err()
}

// documentationHealthPages returns the slug and title projection for one health category query.
func (s *Store) documentationHealthPages(ctx context.Context, statement string, args ...any) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pages []domain.Page
	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.Slug, &page.Title); err != nil {
			return nil, err
		}

		pages = append(pages, page)
	}

	return pages, rows.Err()
}
