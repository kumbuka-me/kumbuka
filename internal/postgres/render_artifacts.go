package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/sdk/renderartifact"
)

// PutPageSVG commits a bounded image only if the referenced page revision is still current.
func (s *Store) PutPageSVG(ctx context.Context, pageID int64, revision time.Time, data []byte) (string, error) {
	if err := renderartifact.Validate(renderartifact.SVGMediaType, data); err != nil {
		return "", err
	}
	digest := renderartifact.Digest(data)
	const insert = `
INSERT INTO page_render_artifacts (page_id, page_updated_at, digest, media_type, data)
SELECT p.id, p.updated_at, $3, $4, $5
FROM pages p
WHERE p.id = $1 AND p.updated_at = $2 AND p.deleted_at IS NULL
ON CONFLICT (page_id, page_updated_at, digest) DO UPDATE SET digest = EXCLUDED.digest
RETURNING digest`
	var committed string
	if err := s.pool.QueryRow(ctx, insert, pageID, revision, digest, renderartifact.SVGMediaType, data).Scan(&committed); err != nil {
		return "", err
	}
	return committed, nil
}

// GetPageSVG reads an image only when the currently authorized page revision still matches.
func (s *Store) GetPageSVG(ctx context.Context, pageID int64, digest string) ([]byte, bool, error) {
	const query = `
SELECT a.data
FROM page_render_artifacts a
JOIN pages p ON p.id=a.page_id AND p.updated_at=a.page_updated_at
WHERE a.page_id=$1 AND a.digest=$2 AND p.deleted_at IS NULL`
	var data []byte
	err := s.pool.QueryRow(ctx, query, pageID, digest).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}
