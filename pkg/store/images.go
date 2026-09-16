package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// SaveImage stores an immutable uploaded image and returns its metadata.
func (s *Store) SaveImage(ctx context.Context, filename, contentType string, data []byte, userID int64) (domain.Image, error) {
	var image domain.Image
	err := s.pool.QueryRow(ctx, `
INSERT INTO images(filename,content_type,data,size_bytes,uploaded_by)
VALUES($1,$2,$3,$4,$5)
RETURNING id,filename,content_type,size_bytes,coalesce(uploaded_by,0),created_at`, filename, contentType, data, int64(len(data)), userID).Scan(
		&image.ID,
		&image.Filename,
		&image.ContentType,
		&image.SizeBytes,
		&image.UploadedBy,
		&image.CreatedAt,
	)
	if err != nil {
		return domain.Image{}, err
	}

	return image, nil
}

// Images returns all uploaded image metadata with exact Markdown reference counts.
func (s *Store) Images(ctx context.Context) ([]domain.Image, error) {
	return s.searchImages(ctx, 0, "", 0, 0)
}

// ImagesByUser returns all image metadata uploaded by one user.
func (s *Store) ImagesByUser(ctx context.Context, userID int64) ([]domain.Image, error) {
	return s.searchImages(ctx, userID, "", 0, 0)
}

// SearchImages returns a bounded slice of images matching filename or uploader.
func (s *Store) SearchImages(ctx context.Context, query string, limit, offset int) ([]domain.Image, error) {
	return s.searchImages(ctx, 0, query, limit, offset)
}

// SearchImagesByUser returns a bounded slice of one user's images matching filename.
func (s *Store) SearchImagesByUser(
	ctx context.Context,
	userID int64,
	query string,
	limit, offset int,
) ([]domain.Image, error) {
	return s.searchImages(ctx, userID, query, limit, offset)
}

// searchImages queries image metadata. A zero user ID selects every uploader and a zero limit is unbounded.
func (s *Store) searchImages(
	ctx context.Context,
	userID int64,
	query string,
	limit, offset int,
) ([]domain.Image, error) {
	rows, err := s.pool.Query(ctx, `
WITH image_references AS (
  SELECT m.captures[1]::bigint AS image_id,count(*) AS usage_count
  FROM pages p
  CROSS JOIN LATERAL regexp_matches(p.markdown_content, '/media/([0-9]+)/', 'g') AS m(captures)
  GROUP BY m.captures[1]::bigint
)
SELECT
  i.id,
  i.filename,
  i.content_type,
  i.size_bytes,
  coalesce(i.uploaded_by,0),
  coalesce(u.display_name,u.username,''),
  i.created_at,
  coalesce(refs.usage_count,0)
FROM images i
LEFT JOIN users u ON u.id=i.uploaded_by
LEFT JOIN image_references refs ON refs.image_id=i.id
WHERE ($1::bigint=0 OR i.uploaded_by=$1)
  AND (
    $2::text=''
    OR position(lower($2) in lower(i.filename))>0
    OR ($1::bigint=0 AND position(lower($2) in lower(coalesce(u.display_name,'')))>0)
    OR ($1::bigint=0 AND position(lower($2) in lower(coalesce(u.username,'')))>0)
  )
ORDER BY i.created_at DESC,i.id DESC
LIMIT NULLIF($3,0)
OFFSET $4`, userID, query, limit, offset)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var images []domain.Image

	for rows.Next() {
		var image domain.Image
		if err := rows.Scan(
			&image.ID,
			&image.Filename,
			&image.ContentType,
			&image.SizeBytes,
			&image.UploadedBy,
			&image.Uploader,
			&image.CreatedAt,
			&image.UsageCount,
		); err != nil {
			return nil, err
		}

		images = append(images, image)
	}

	return images, rows.Err()
}

// ImageInfo returns image metadata with its exact current Markdown reference count.
func (s *Store) ImageInfo(ctx context.Context, id int64) (domain.Image, error) {
	var image domain.Image
	err := s.pool.QueryRow(ctx, `
SELECT
  i.id,
  i.filename,
  i.content_type,
  i.size_bytes,
  coalesce(i.uploaded_by,0),
  coalesce(u.display_name,u.username,''),
  i.created_at,
  coalesce((
    SELECT sum(regexp_count(p.markdown_content, '/media/' || i.id::text || '/'))
    FROM pages p
  ),0)
FROM images i
LEFT JOIN users u ON u.id=i.uploaded_by
WHERE i.id=$1`, id).Scan(
		&image.ID,
		&image.Filename,
		&image.ContentType,
		&image.SizeBytes,
		&image.UploadedBy,
		&image.Uploader,
		&image.CreatedAt,
		&image.UsageCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Image{}, domain.ErrNotFound
	}

	return image, err
}

// ImageContent returns the binary payload for one uploaded image.
func (s *Store) ImageContent(ctx context.Context, id int64) (domain.ImageData, error) {
	var image domain.ImageData
	err := s.pool.QueryRow(ctx, `
SELECT filename,content_type,data
FROM images
WHERE id=$1`, id).
		Scan(&image.Filename, &image.ContentType, &image.Data)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ImageData{}, domain.ErrNotFound
	}

	return image, err
}

// DeleteImage permanently removes an uploaded image by identifier.
func (s *Store) DeleteImage(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM images
WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}
