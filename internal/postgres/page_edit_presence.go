package postgres

import (
	"context"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// TouchPageEditor records that one user is actively editing a page.
func (s *Store) TouchPageEditor(ctx context.Context, slug string, userID int64) error {
	tag, err := s.pool.Exec(ctx, `
INSERT INTO page_edit_presence(page_id,user_id,updated_at)
SELECT p.id,$2,now()
FROM pages p
WHERE p.slug=$1 AND p.deleted_at IS NULL
ON CONFLICT(page_id,user_id) DO UPDATE
SET updated_at=EXCLUDED.updated_at`, slug, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return mutationError(err)
}

// LeavePageEditor removes one user's active editor presence for a page.
func (s *Store) LeavePageEditor(ctx context.Context, slug string, userID int64) error {
	_, err := s.pool.Exec(ctx, `
DELETE FROM page_edit_presence presence
USING pages page
WHERE presence.page_id=page.id
  AND page.slug=$1
  AND presence.user_id=$2`, slug, userID)
	return mutationError(err)
}

// PageEditors returns editors whose heartbeat is recent enough to count as active.
func (s *Store) PageEditors(
	ctx context.Context,
	slug string,
	excludeUserID int64,
	activeWithin time.Duration,
) ([]domain.PageEditorPresence, error) {
	rows, err := s.pool.Query(ctx, `
SELECT presence.user_id,coalesce(users.display_name,users.username,''),presence.updated_at
FROM page_edit_presence presence
JOIN pages page ON page.id=presence.page_id
JOIN users ON users.id=presence.user_id
WHERE page.slug=$1
  AND page.deleted_at IS NULL
  AND presence.user_id<>$2
  AND presence.updated_at >= now() - ($3 * interval '1 second')
ORDER BY presence.updated_at DESC,presence.user_id`, slug, excludeUserID, int64(activeWithin/time.Second))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	editors := make([]domain.PageEditorPresence, 0)
	for rows.Next() {
		var editor domain.PageEditorPresence
		if err := rows.Scan(&editor.UserID, &editor.Name, &editor.UpdatedAt); err != nil {
			return nil, err
		}
		editors = append(editors, editor)
	}

	return editors, rows.Err()
}
