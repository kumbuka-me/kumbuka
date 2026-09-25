package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// EnqueuePluginContentChanges persists committed page-source events for durable per-plugin delivery.
func (s *Store) EnqueuePluginContentChanges(ctx context.Context, changes []domain.PluginContentChange) error {
	if len(changes) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, change := range changes {
		page := change.Page
		page.Markdown = ""
		encodedPage, err := json.Marshal(page)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO plugin_content_changes(plugin_id,actor_id,page_snapshot,previous_markdown,markdown)
VALUES($1,$2,$3,$4,$5)`, change.PluginID, change.ActorID, encodedPage, change.PreviousMarkdown, change.Markdown); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ClaimPluginContentChanges leases ready events so multiple Kumbuka instances do not deliver the same row concurrently.
func (s *Store) ClaimPluginContentChanges(ctx context.Context, limit int, lease time.Duration) ([]domain.PluginContentChange, error) {
	if limit <= 0 {
		return nil, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
WITH candidates AS (
    SELECT id
    FROM plugin_content_changes
    WHERE available_at <= now()
      AND (claimed_until IS NULL OR claimed_until < now())
    ORDER BY id
    FOR UPDATE SKIP LOCKED
    LIMIT $1
)
UPDATE plugin_content_changes AS queued
SET claimed_until = now() + ($2::bigint * interval '1 millisecond'),
    attempts = queued.attempts + 1
FROM candidates
WHERE queued.id = candidates.id
RETURNING queued.id,queued.plugin_id,queued.page_snapshot,queued.previous_markdown,queued.markdown,queued.actor_id,queued.attempts`, limit, lease.Milliseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := make([]domain.PluginContentChange, 0, limit)
	for rows.Next() {
		var (
			change domain.PluginContentChange
			page   []byte
		)
		if err := rows.Scan(&change.ID, &change.PluginID, &page, &change.PreviousMarkdown, &change.Markdown, &change.ActorID, &change.Attempts); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(page, &change.Page); err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return changes, nil
}

// CompletePluginContentChange removes one successfully delivered queue event.
func (s *Store) CompletePluginContentChange(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM plugin_content_changes WHERE id=$1`, id)
	return err
}

// RetryPluginContentChange releases one failed event for a later delivery attempt.
func (s *Store) RetryPluginContentChange(ctx context.Context, id int64, availableAt time.Time, lastError string) error {
	_, err := s.pool.Exec(ctx, `
UPDATE plugin_content_changes
SET available_at=$2,claimed_until=NULL,last_error=$3
WHERE id=$1`, id, availableAt, lastError)
	return err
}
