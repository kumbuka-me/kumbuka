package postgres

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageLinks returns outgoing wiki links and whether their targets currently exist.
func (s *Store) PageLinks(ctx context.Context, slug string) ([]domain.PageLink, error) {
	rows, err := s.pool.Query(ctx, `
SELECT l.target_slug,coalesce(target.title,alias_target.title,''),(target.id IS NOT NULL OR alias_target.id IS NOT NULL)
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
		if err := rows.Scan(&link.TargetSlug, &link.TargetTitle, &link.Exists); err != nil {
			return nil, err
		}

		links = append(links, link)
	}

	return links, rows.Err()
}
