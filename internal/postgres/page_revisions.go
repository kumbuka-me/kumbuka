package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// LatestRevision returns the newest revision and total count, or a zero count when none exist.
func (s *Store) LatestRevision(ctx context.Context, slug string) (record revision.Revision, count int, err error) {
	const query = `
SELECT
  r.id,
  r.revision_number,
  coalesce(u.display_name,u.username,''),
  r.created_at,
  r.message,
  r.markdown_content,
  coalesce((SELECT previous.markdown_content FROM page_revisions previous WHERE previous.page_id=r.page_id AND previous.revision_number=r.revision_number-1),''),
  count(*) OVER()
FROM page_revisions r
JOIN pages p ON p.id=r.page_id
LEFT JOIN users u ON u.id=r.created_by
WHERE p.slug=$1 AND p.deleted_at IS NULL
ORDER BY r.revision_number DESC
LIMIT 1`

	var markdown, previous string
	err = s.pool.QueryRow(ctx, query, slug).Scan(
		&record.ID,
		&record.Number,
		&record.Author,
		&record.CreatedAt,
		&record.Message,
		&markdown,
		&previous,
		&count,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return revision.Revision{}, 0, nil
	}

	if err == nil {
		record.Markdown = markdown
		record.PreviousMarkdown = previous
	}

	return record, count, err
}

// Revision returns one persisted page revision by revision number.
func (s *Store) Revision(ctx context.Context, slug string, number int) (revision.Revision, error) {
	var record revision.Revision
	err := s.pool.QueryRow(ctx, `
SELECT r.id,r.revision_number,coalesce(u.display_name,u.username,''),r.created_at,r.message,r.markdown_content
FROM page_revisions r
JOIN pages p ON p.id=r.page_id
LEFT JOIN users u ON u.id=r.created_by
WHERE p.slug=$1 AND p.deleted_at IS NULL AND r.revision_number=$2`, slug, number).Scan(
		&record.ID,
		&record.Number,
		&record.Author,
		&record.CreatedAt,
		&record.Message,
		&record.Markdown,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return revision.Revision{}, domain.ErrRevisionNotFound
	}

	return record, err
}

// Revisions returns a page's revision metadata in newest-first order.
func (s *Store) Revisions(ctx context.Context, slug string) ([]revision.Revision, error) {
	const query = `
WITH history AS (
  SELECT
    r.id,
    r.revision_number,
    r.created_by,
    r.created_at,
    r.message,
    r.markdown_content,
    lag(r.markdown_content,1,'') OVER (ORDER BY r.revision_number) AS previous_markdown
  FROM page_revisions r
  JOIN pages p ON p.id=r.page_id
  WHERE p.slug=$1 AND p.deleted_at IS NULL
)
SELECT h.id,h.revision_number,coalesce(u.display_name,u.username,''),h.created_at,h.message,h.markdown_content,h.previous_markdown
FROM history h
LEFT JOIN users u ON u.id=h.created_by
ORDER BY h.revision_number DESC`

	rows, err := s.pool.Query(ctx, query, slug)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var revisions []revision.Revision

	for rows.Next() {
		var record revision.Revision
		if err := rows.Scan(
			&record.ID,
			&record.Number,
			&record.Author,
			&record.CreatedAt,
			&record.Message,
			&record.Markdown,
			&record.PreviousMarkdown,
		); err != nil {
			return nil, err
		}

		revisions = append(revisions, record)
	}

	return revisions, rows.Err()
}

// TaggedPages returns page slugs and tags for access-aware tag discovery.
func (s *Store) TaggedPages(ctx context.Context) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.slug,array_agg(t.name ORDER BY t.name)
FROM pages p
JOIN page_tags pt ON pt.page_id=p.id
JOIN tags t ON t.id=pt.tag_id
WHERE p.deleted_at IS NULL
GROUP BY p.id
ORDER BY p.slug`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.Page
	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.Slug, &page.Tags); err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}

	return pages, rows.Err()
}
