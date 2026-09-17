package store

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Stats returns high-level database counts for administrators.
func (s *Store) Stats(ctx context.Context) (domain.AdminStats, error) {
	var stats domain.AdminStats
	err := s.pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM users),
  (SELECT count(*) FROM wiki_groups),
  (SELECT count(*) FROM pages WHERE deleted_at IS NULL),
  (SELECT count(*) FROM pages WHERE deleted_at IS NOT NULL),
  (SELECT count(*) FROM tags),
  (SELECT count(*) FROM images),
  (SELECT count(*) FROM api_tokens)`).Scan(
		&stats.Users,
		&stats.Groups,
		&stats.Pages,
		&stats.DeletedPages,
		&stats.Tags,
		&stats.Images,
		&stats.Tokens,
	)

	return stats, err
}
