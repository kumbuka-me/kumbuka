package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// PageSlugByID resolves the current slug for a live page identifier.
func (s *Store) PageSlugByID(ctx context.Context, id int64) (string, error) {
	var slug string
	err := s.pool.QueryRow(ctx, `
SELECT slug
FROM pages
WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}

	return slug, err
}
