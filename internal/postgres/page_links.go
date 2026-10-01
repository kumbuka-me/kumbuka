package postgres

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// ResolvePageLinks resolves requested current or historical slugs to stable page identities.
func (s *Store) ResolvePageLinks(ctx context.Context, slugs []string) ([]domain.PageLink, error) {
	rows, err := s.pool.Query(ctx, `
SELECT
  requested.slug,
  coalesce(target.id,alias_target.id,0),
  coalesce(target.slug,alias_target.slug,''),
  coalesce(target.title,alias_target.title,''),
  (target.id IS NOT NULL OR alias_target.id IS NOT NULL)
FROM unnest($1::text[]) AS requested(slug)
LEFT JOIN pages target ON target.slug=requested.slug AND target.deleted_at IS NULL
LEFT JOIN page_aliases alias ON alias.alias=requested.slug
LEFT JOIN pages alias_target ON alias_target.id=alias.page_id AND alias_target.deleted_at IS NULL
ORDER BY requested.slug`, slugs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []domain.PageLink
	for rows.Next() {
		var link domain.PageLink
		if err := rows.Scan(&link.TargetSlug, &link.TargetID, &link.ResolvedSlug, &link.TargetTitle, &link.Exists); err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, rows.Err()
}

// PageLinks returns outgoing wiki links and whether their targets currently exist.
func (s *Store) PageLinks(ctx context.Context, slug string) ([]domain.PageLink, error) {
	rows, err := s.pool.Query(ctx, `
SELECT
  l.target_slug,
  coalesce(target.id,alias_target.id,0),
  coalesce(target.slug,alias_target.slug,''),
  coalesce(target.title,alias_target.title,''),
  (target.id IS NOT NULL OR alias_target.id IS NOT NULL)
FROM page_links l
JOIN pages source ON source.id=l.source_page_id AND source.deleted_at IS NULL
LEFT JOIN pages target ON target.slug=l.target_slug AND target.deleted_at IS NULL
LEFT JOIN page_aliases alias ON alias.alias=l.target_slug
LEFT JOIN pages alias_target ON alias_target.id=alias.page_id AND alias_target.deleted_at IS NULL
WHERE source.slug=$1
ORDER BY l.target_slug`, slug)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var links []domain.PageLink

	for rows.Next() {
		var link domain.PageLink
		if err := rows.Scan(&link.TargetSlug, &link.TargetID, &link.ResolvedSlug, &link.TargetTitle, &link.Exists); err != nil {
			return nil, err
		}

		links = append(links, link)
	}

	return links, rows.Err()
}
