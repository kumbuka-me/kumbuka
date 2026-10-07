package postgres

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// DeletePage moves a page into the recycle bin.
func (s *Store) DeletePage(ctx context.Context, slug string, userID int64) error {
	tag, err := s.pool.Exec(
		ctx,
		`
UPDATE pages
SET deleted_at=now(),deleted_by=$2
WHERE slug=$1 AND deleted_at IS NULL`,
		slug,
		userID,
	)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// DeletedPages returns pages currently held in the recycle bin, newest deletion first.
func (s *Store) DeletedPages(ctx context.Context) ([]domain.DeletedPage, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.id,p.slug,p.title,coalesce(max(ni.icon),''),p.markdown_content,coalesce(p.created_by,0),coalesce(p.updated_by,0),
       coalesce(editor.display_name,editor.username,''),p.created_at,p.updated_at,p.view_count,
       coalesce(array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),'{}'),
       p.deleted_at,coalesce(deleter.display_name,deleter.username,'')
FROM pages p
LEFT JOIN navigation_icons ni ON ni.path=p.slug
LEFT JOIN users editor ON editor.id=p.updated_by
LEFT JOIN users deleter ON deleter.id=p.deleted_by
LEFT JOIN page_tags pt ON pt.page_id=p.id
LEFT JOIN tags t ON t.id=pt.tag_id
WHERE p.deleted_at IS NOT NULL
GROUP BY p.id,editor.id,deleter.id
ORDER BY p.deleted_at DESC,p.id DESC`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.DeletedPage

	for rows.Next() {
		var item domain.DeletedPage
		if err := rows.Scan(&item.ID, &item.Slug, &item.Title, &item.Icon, &item.Markdown, &item.CreatedBy, &item.UpdatedBy, &item.Author, &item.CreatedAt, &item.UpdatedAt, &item.ViewCount, &item.Tags, &item.DeletedAt, &item.DeletedBy); err != nil {
			return nil, err
		}

		pages = append(pages, item)
	}

	return pages, rows.Err()
}

// RestorePage restores a page from the recycle bin.
func (s *Store) RestorePage(ctx context.Context, slug string) error {
	tag, err := s.pool.Exec(
		ctx,
		`
UPDATE pages
SET deleted_at=NULL,deleted_by=NULL
WHERE slug=$1 AND deleted_at IS NOT NULL`,
		slug,
	)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return mutationError(err)
}

// PermanentlyDeletePage removes one page already held in the recycle bin.
func (s *Store) PermanentlyDeletePage(ctx context.Context, slug string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
DELETE FROM pages
WHERE slug=$1
  AND deleted_at IS NOT NULL`, slug)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM navigation_icons AS icon
WHERE NOT EXISTS (
  SELECT 1
  FROM pages
  WHERE slug=icon.path
     OR slug LIKE icon.path || '/%'
)`); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
