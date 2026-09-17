package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageComments returns comments for a page, unresolved first.
func (s *Store) PageComments(ctx context.Context, slug string) ([]domain.PageComment, error) {
	rows, err := s.pool.Query(ctx, `
SELECT c.id,c.page_id,coalesce(c.parent_id,0),coalesce(parent_user.display_name,parent_user.username,'Deleted user'),
       coalesce(parent.body,''),coalesce(u.display_name,u.username,'Deleted user'),c.anchor,c.quote,c.body,c.resolved_at,c.created_at
FROM page_comments c
JOIN pages p ON p.id=c.page_id
LEFT JOIN users u ON u.id=c.user_id
LEFT JOIN page_comments parent ON parent.id=c.parent_id
LEFT JOIN users parent_user ON parent_user.id=parent.user_id
WHERE p.slug=$1 AND p.deleted_at IS NULL
ORDER BY (c.resolved_at IS NOT NULL),c.created_at`, slug)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var items []domain.PageComment

	for rows.Next() {
		var item domain.PageComment
		if err := rows.Scan(
			&item.ID,
			&item.PageID,
			&item.ParentID,
			&item.ParentAuthor,
			&item.ParentBody,
			&item.Author,
			&item.Anchor,
			&item.Quote,
			&item.Body,
			&item.Resolved,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// AddPageComment adds a comment to a page.
func (s *Store) AddPageComment(
	ctx context.Context, slug string, userID, parentID int64, anchor, quote, body string,
) (domain.PageComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.PageComment{}, domain.NewValidationError("body", "A comment is required.")
	}

	var item domain.PageComment
	err := s.pool.QueryRow(ctx, `
INSERT INTO page_comments(page_id,user_id,parent_id,anchor,quote,body)
SELECT p.id,$2,nullif($3,0),$4,$5,$6
FROM pages p
WHERE p.slug=$1
  AND p.deleted_at IS NULL
  AND ($3=0 OR EXISTS (
    SELECT 1 FROM page_comments parent WHERE parent.id=$3 AND parent.page_id=p.id
  ))
RETURNING id,page_id,coalesce(parent_id,0),$7,anchor,quote,body,resolved_at,created_at`,
		slug, userID, parentID, strings.TrimSpace(anchor), strings.TrimSpace(quote), body, "").
		Scan(
			&item.ID, &item.PageID, &item.ParentID, &item.Author, &item.Anchor,
			&item.Quote, &item.Body, &item.Resolved, &item.CreatedAt,
		)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageComment{}, domain.ErrNotFound
	}

	return item, err
}

// ResolvePageComment resolves or reopens one page comment.
func (s *Store) ResolvePageComment(ctx context.Context, id int64, resolved bool) error {
	var value any

	if resolved {
		value = time.Now()
	}

	tag, err := s.pool.Exec(ctx, `
UPDATE page_comments
SET resolved_at=$2,updated_at=now()
WHERE id=$1`, id, value)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrCommentNotFound
	}

	return err
}
