package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Notifications returns a user's newest notifications and unread count.
func (s *Store) Notifications(
	ctx context.Context,
	userID int64,
	limit int,
) (notifications []domain.Notification, unread int, err error) {
	if err := s.pool.QueryRow(ctx, `
SELECT count(*)
FROM notifications
WHERE user_id=$1 AND read_at IS NULL`, userID).Scan(&unread); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
SELECT id,kind,title,body,url,user_id,coalesce(actor_id,0),source_type,source_id,source_name,read_at,created_at
FROM notifications
WHERE user_id=$1
ORDER BY created_at DESC,id DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, 0, err
	}

	defer rows.Close()

	for rows.Next() {
		var item domain.Notification
		if err := rows.Scan(
			&item.ID,
			&item.Kind,
			&item.Title,
			&item.Body,
			&item.URL,
			&item.RecipientUserID,
			&item.ActorID,
			&item.SourceType,
			&item.SourceID,
			&item.SourceName,
			&item.ReadAt,
			&item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		notifications = append(notifications, item)
	}

	return notifications, unread, rows.Err()
}

// CreateNotification inserts one notification and resolves idempotent plugin retries to the existing row.
func (s *Store) CreateNotification(ctx context.Context, item domain.Notification) (domain.Notification, bool, error) {
	var created bool
	err := s.pool.QueryRow(ctx, `
INSERT INTO notifications(user_id,kind,title,body,url,actor_id,source_type,source_id,source_name,idempotency_key)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
ON CONFLICT (source_id,user_id,idempotency_key)
WHERE source_type='plugin' AND idempotency_key<>''
DO UPDATE SET id=notifications.id
RETURNING id,user_id,kind,title,body,url,coalesce(actor_id,0),source_type,source_id,source_name,idempotency_key,read_at,created_at,(xmax=0)`,
		item.RecipientUserID,
		item.Kind,
		item.Title,
		item.Body,
		item.URL,
		item.ActorID,
		item.SourceType,
		item.SourceID,
		item.SourceName,
		item.IdempotencyKey,
	).Scan(
		&item.ID,
		&item.RecipientUserID,
		&item.Kind,
		&item.Title,
		&item.Body,
		&item.URL,
		&item.ActorID,
		&item.SourceType,
		&item.SourceID,
		&item.SourceName,
		&item.IdempotencyKey,
		&item.ReadAt,
		&item.CreatedAt,
		&created,
	)
	return item, created, err
}

// MarkNotificationRead marks one owned notification read without exposing foreign identifiers.
func (s *Store) MarkNotificationRead(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx, `
UPDATE notifications
SET read_at=coalesce(read_at,now())
WHERE id=$1 AND user_id=$2`, id, userID)

	return err
}

// MarkNotificationUnread marks one owned notification unread without exposing foreign identifiers.
func (s *Store) MarkNotificationUnread(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx, `
UPDATE notifications
SET read_at=NULL
WHERE id=$1 AND user_id=$2`, id, userID)

	return err
}

// MarkAllNotificationsRead marks every notification owned by a user as read.
func (s *Store) MarkAllNotificationsRead(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `
UPDATE notifications
SET read_at=coalesce(read_at,now())
WHERE user_id=$1`, userID)

	return err
}

// DeleteNotification removes one owned notification without exposing foreign identifiers.
func (s *Store) DeleteNotification(ctx context.Context, userID, id int64) error {
	_, err := s.pool.Exec(ctx, `
DELETE FROM notifications
WHERE id=$1 AND user_id=$2`, id, userID)

	return err
}

// OpenNotification atomically marks an owned item read and returns its destination.
func (s *Store) OpenNotification(ctx context.Context, userID, id int64) (string, error) {
	var destination string
	err := s.pool.QueryRow(ctx, `
UPDATE notifications
SET read_at=coalesce(read_at,now())
WHERE id=$1 AND user_id=$2
RETURNING url`, id, userID).Scan(&destination)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}

	return destination, err
}
