package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// BrandLogo returns the configured instance-wide brand logo.
func (s *Store) BrandLogo(ctx context.Context) (contentType string, data []byte, err error) {
	err = s.pool.QueryRow(ctx, `
SELECT brand_logo_content_type,brand_logo_data
FROM application_settings
WHERE singleton=true AND brand_logo_data IS NOT NULL`).Scan(&contentType, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, domain.ErrNotFound
	}

	return contentType, data, err
}

// SaveBrandLogo replaces the configured instance-wide brand logo.
func (s *Store) SaveBrandLogo(ctx context.Context, contentType string, data []byte) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE application_settings
SET brand_logo_content_type=$1,
    brand_logo_data=$2,
    updated_at=now()
WHERE singleton=true`, contentType, data)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// ClearBrandLogo restores the built-in brand logo.
func (s *Store) ClearBrandLogo(ctx context.Context) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE application_settings
SET brand_logo_content_type='',
    brand_logo_data=NULL,
    updated_at=now()
WHERE singleton=true`)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}
