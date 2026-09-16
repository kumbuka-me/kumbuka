package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// CreatePageShareLink stores a hashed public permalink token for one page.
func (s *Store) CreatePageShareLink(
	ctx context.Context,
	pageID, createdBy int64,
	tokenHash string,
) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO page_share_links(page_id,token_hash,created_by)
VALUES($1,$2,$3)`, pageID, tokenHash, createdBy)
	return err
}

// PageShareLink resolves an active public permalink token.
func (s *Store) PageShareLink(ctx context.Context, tokenHash string) (domain.PageShareLink, error) {
	var link domain.PageShareLink
	err := s.pool.QueryRow(ctx, `
SELECT l.page_id,p.slug,p.title
FROM page_share_links l
JOIN pages p ON p.id=l.page_id
WHERE l.token_hash=$1
  AND l.revoked_at IS NULL
  AND p.deleted_at IS NULL`, tokenHash).Scan(&link.PageID, &link.Slug, &link.Title)

	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	}

	return link, err
}
