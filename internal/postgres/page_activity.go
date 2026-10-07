package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// RecordView increments page views and records the user's most recent view.
func (s *Store) RecordView(ctx context.Context, slug string, userID int64) error {
	_, err := s.pool.Exec(
		ctx,
		`
WITH p AS (UPDATE pages SET view_count=view_count+1 WHERE slug=$1 AND deleted_at IS NULL RETURNING id) INSERT INTO page_views(user_id,page_id,viewed_at) SELECT $2,id,now()
FROM p
ON CONFLICT(user_id,page_id) DO UPDATE
SET viewed_at=now()`,
		slug,
		userID,
	)
	return err
}

// SetFavorite adds or removes a page from a user's favorites.
func (s *Store) SetFavorite(ctx context.Context, slug string, userID int64, on bool) error {
	if on {
		_, err := s.pool.Exec(
			ctx,
			`
INSERT INTO favorites(user_id,page_id)
SELECT $2,id FROM pages
WHERE slug=$1 AND deleted_at IS NULL
ON CONFLICT DO NOTHING`,
			slug,
			userID,
		)
		return err
	}

	_, err := s.pool.Exec(
		ctx,
		`
DELETE FROM favorites f
USING pages p
WHERE f.page_id=p.id AND p.slug=$1 AND p.deleted_at IS NULL AND f.user_id=$2`,
		slug,
		userID,
	)

	return err
}

// IsFavorite reports whether a page is currently pinned as a favorite by the user.
func (s *Store) IsFavorite(ctx context.Context, slug string, userID int64) (bool, error) {
	var favorite bool
	err := s.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM favorites f
  JOIN pages p ON p.id=f.page_id
  WHERE f.user_id=$2 AND p.slug=$1 AND p.deleted_at IS NULL
)`, slug, userID).Scan(&favorite)

	return favorite, err
}

// Favorites returns a user's favorite pages in newest-first order.
func (s *Store) Favorites(ctx context.Context, userID int64) ([]domain.Page, error) {
	rows, err := s.pool.Query(
		ctx,
		pageSelect+`
JOIN favorites f ON f.page_id=p.id AND f.user_id=$1
WHERE p.deleted_at IS NULL
GROUP BY p.id,u.id,f.created_at
ORDER BY f.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return collectPages(rows)
}

// Backlinks returns pages that reference the supplied page slug.
func (s *Store) Backlinks(ctx context.Context, slug string) ([]domain.Page, error) {
	base := slug
	if _, segment, ok := strings.CutLast(slug, "/"); ok {
		base = segment
	}

	rows, err := s.pool.Query(
		ctx,
		pageSelect+`
JOIN page_links l ON l.source_page_id=p.id AND l.target_slug IN ($1,$2)
WHERE p.deleted_at IS NULL
GROUP BY p.id,u.id
ORDER BY p.title`,
		slug,
		base,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return collectPages(rows)
}

// ResolvePageAlias resolves a historical page path to its current active slug.
func (s *Store) ResolvePageAlias(ctx context.Context, alias string) (string, error) {
	var slug string
	err := s.pool.QueryRow(ctx, `
SELECT p.slug
FROM page_aliases a
JOIN pages p ON p.id=a.page_id AND p.deleted_at IS NULL
WHERE a.alias=$1`, alias).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}

	return slug, err
}
