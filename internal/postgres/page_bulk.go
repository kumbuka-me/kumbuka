package postgres

import (
	"context"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageInventory returns pages with lifecycle metadata for administration.
func (s *Store) PageInventory(ctx context.Context) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.id,p.slug,p.title,p.status,coalesce(p.owner_group_id,0),coalesce(g.name,''),p.last_reviewed_at,p.review_interval_days,p.updated_at
FROM pages p
LEFT JOIN wiki_groups g ON g.id=p.owner_group_id
WHERE p.deleted_at IS NULL
ORDER BY p.slug`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.Page

	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.ID, &page.Slug, &page.Title, &page.Status, &page.OwnerGroupID, &page.OwnerGroup, &page.LastReviewedAt, &page.ReviewIntervalDays, &page.UpdatedAt); err != nil {
			return nil, err
		}

		pages = append(pages, page)
	}

	return pages, rows.Err()
}

// BulkSetPageStatus updates lifecycle status for selected pages.
func (s *Store) BulkSetPageStatus(ctx context.Context, slugs []string, status domain.PageStatus) error {
	if !domain.ValidPageStatus(status) {
		return domain.NewValidationError("status", "Choose a valid page status.")
	}

	_, err := s.pool.Exec(ctx, `
UPDATE pages
SET status=$2,updated_at=now()
WHERE slug=ANY($1::text[]) AND deleted_at IS NULL`, slugs, string(status))

	return err
}

// BulkAddPageTag adds one normalized tag to selected pages.
func (s *Store) BulkAddPageTag(ctx context.Context, slugs []string, tag string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return domain.NewValidationError("tag", "A tag is required.")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var tagID int64
	if err := tx.QueryRow(ctx, `
INSERT INTO tags(name)
VALUES($1)
ON CONFLICT(name) DO UPDATE
SET name=EXCLUDED.name
RETURNING id`, tag).Scan(&tagID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO page_tags(page_id,tag_id)
SELECT id,$2 FROM pages
WHERE slug=ANY($1::text[]) AND deleted_at IS NULL
ON CONFLICT DO NOTHING`, slugs, tagID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// BulkAssignPageGroup assigns one collaboration group to selected pages.
func (s *Store) BulkAssignPageGroup(ctx context.Context, slugs []string, groupID int64) error {
	if groupID <= 0 {
		return domain.NewValidationError("group_id", "Choose a valid group.")
	}

	_, err := s.pool.Exec(ctx, `
INSERT INTO page_groups(page_id,group_id)
SELECT id,$2 FROM pages
WHERE slug=ANY($1::text[]) AND deleted_at IS NULL
ON CONFLICT DO NOTHING`, slugs, groupID)

	return mutationError(err)
}

// BulkDeletePages moves selected pages to the recycle bin.
func (s *Store) BulkDeletePages(ctx context.Context, slugs []string, userID int64) error {
	_, err := s.pool.Exec(ctx, `
UPDATE pages
SET deleted_at=now(),deleted_by=$2
WHERE slug=ANY($1::text[]) AND deleted_at IS NULL`, slugs, userID)
	return err
}

// PageAliases returns persisted redirect aliases and their current target slugs.
func (s *Store) PageAliases(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT a.alias,p.slug
FROM page_aliases a
JOIN pages p ON p.id=a.page_id
WHERE p.deleted_at IS NULL
ORDER BY a.alias`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	aliases := map[string]string{}

	for rows.Next() {
		var alias, target string
		if err := rows.Scan(&alias, &target); err != nil {
			return nil, err
		}

		aliases[alias] = target
	}

	return aliases, rows.Err()
}
