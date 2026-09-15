package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/internal/domain"
)

type pageDraftScanner interface {
	Scan(...any) error
}

const pageDraftSelect = `
SELECT
  d.id,
  d.draft_key,
  coalesce(d.page_id,0),
  d.base_revision,
  coalesce((SELECT max(r.revision_number) FROM page_revisions r WHERE r.page_id=d.page_id),0),
  d.title,
  d.slug,
  coalesce(p.slug,''),
  d.form_values,
  d.created_at,
  d.updated_at
FROM page_drafts d
LEFT JOIN pages p ON p.id=d.page_id`

// PageDraft returns one private draft owned by a user.
func (s *Store) PageDraft(ctx context.Context, userID int64, key string) (domain.PageDraft, error) {
	return scanPageDraft(s.pool.QueryRow(
		ctx,
		pageDraftSelect+`
WHERE d.user_id=$1 AND d.draft_key=$2 AND (d.page_id IS NULL OR p.deleted_at IS NULL)`,
		userID,
		key,
	))
}

// PageDrafts returns one user's private drafts newest first without loading their full form payloads.
func (s *Store) PageDrafts(ctx context.Context, userID int64, limit int) ([]domain.PageDraft, error) {
	rows, err := s.pool.Query(ctx, `
SELECT
  d.id,
  d.draft_key,
  coalesce(d.page_id,0),
  d.base_revision,
  coalesce((SELECT max(r.revision_number) FROM page_revisions r WHERE r.page_id=d.page_id),0),
  d.title,
  d.slug,
  coalesce(p.slug,''),
  '{}'::jsonb,
  d.created_at,
  d.updated_at
FROM page_drafts d
LEFT JOIN pages p ON p.id=d.page_id
WHERE d.user_id=$1 AND (d.page_id IS NULL OR p.deleted_at IS NULL)
ORDER BY d.updated_at DESC,d.id DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	result := make([]domain.PageDraft, 0)

	for rows.Next() {
		draft, err := scanPageDraft(rows)
		if err != nil {
			return nil, err
		}

		result = append(result, draft)
	}

	return result, rows.Err()
}

// SavePageDraft upserts one user's private editor state without mutating the page or revision history.
func (s *Store) SavePageDraft(
	ctx context.Context,
	userID int64,
	key string,
	pageID int64,
	title string,
	slug string,
	values map[string][]string,
) (domain.PageDraft, error) {
	payload, err := json.Marshal(values)
	if err != nil {
		return domain.PageDraft{}, err
	}

	baseRevision := 0

	if pageID > 0 {
		err := s.pool.QueryRow(ctx, `
SELECT coalesce(max(r.revision_number),0)
FROM pages p
LEFT JOIN page_revisions r ON r.page_id=p.id
WHERE p.id=$1 AND p.deleted_at IS NULL
GROUP BY p.id`, pageID).Scan(&baseRevision)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PageDraft{}, domain.ErrNotFound
		}
		if err != nil {
			return domain.PageDraft{}, err
		}
	}

	_, err = s.pool.Exec(ctx, `
INSERT INTO page_drafts(user_id,draft_key,page_id,base_revision,title,slug,form_values)
VALUES($1,$2,NULLIF($3,0),$4,$5,$6,$7::jsonb)
ON CONFLICT(user_id,draft_key) DO UPDATE SET
  page_id=EXCLUDED.page_id,
  title=EXCLUDED.title,
  slug=EXCLUDED.slug,
  form_values=EXCLUDED.form_values,
  updated_at=now()`, userID, key, pageID, baseRevision, title, slug, string(payload))
	if err != nil {
		return domain.PageDraft{}, err
	}

	return s.PageDraft(ctx, userID, key)
}

// DeletePageDraft removes one private draft if it exists.
func (s *Store) DeletePageDraft(ctx context.Context, userID int64, key string) error {
	_, err := s.pool.Exec(ctx, `
DELETE FROM page_drafts
WHERE user_id=$1 AND draft_key=$2`, userID, key)
	return err
}

// scanPageDraft scans a private draft and decodes its generic form payload.
func scanPageDraft(row pageDraftScanner) (domain.PageDraft, error) {
	var draft domain.PageDraft
	var payload []byte
	err := row.Scan(
		&draft.ID,
		&draft.Key,
		&draft.PageID,
		&draft.BaseRevision,
		&draft.CurrentRevision,
		&draft.Title,
		&draft.Slug,
		&draft.PageSlug,
		&payload,
		&draft.CreatedAt,
		&draft.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageDraft{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PageDraft{}, err
	}

	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &draft.Values); err != nil {
			return domain.PageDraft{}, err
		}
	}

	draft.Stale = draft.PageID > 0 && draft.CurrentRevision > draft.BaseRevision

	return draft, nil
}
