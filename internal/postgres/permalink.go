package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageByID resolves the current page for a live stable identifier.
func (s *Store) PageByID(ctx context.Context, id int64) (domain.Page, error) {
	var page domain.Page
	err := s.pool.QueryRow(ctx, `
SELECT id,slug,title
FROM pages
WHERE id=$1 AND deleted_at IS NULL`, id).Scan(&page.ID, &page.Slug, &page.Title)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Page{}, domain.ErrNotFound
	}

	return page, err
}
