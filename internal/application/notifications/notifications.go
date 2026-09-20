package notifications

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const (
	defaultNotificationListSize = 30
	maxNotificationListSize     = 100
)

// notificationRepository contains persistence operations for the notification inbox.
type notificationRepository interface {
	Notifications(context.Context, int64, int) (notifications []domain.Notification, unread int, err error)
	MarkNotificationRead(context.Context, int64, int64) error
	MarkAllNotificationsRead(context.Context, int64) error
	OpenNotification(context.Context, int64, int64) (string, error)
}

// Notifications exposes per-user notification inbox use cases.
type Notifications struct {
	// repository provides the persistence operations required by notifications.
	repository notificationRepository
}

// NewNotifications constructs the notification inbox service.
func NewNotifications(repository notificationRepository) *Notifications {
	return &Notifications{repository: repository}
}

// Notifications returns a user's recent notifications and unread count.
func (s *Notifications) Notifications(
	ctx context.Context,
	userID int64,
	limit int,
) (notifications []domain.Notification, unread int, err error) {
	if limit <= 0 {
		limit = defaultNotificationListSize
	}
	limit = min(limit, maxNotificationListSize)

	return s.repository.Notifications(ctx, userID, limit)
}

// MarkNotificationRead marks one owned notification as read.
func (s *Notifications) MarkNotificationRead(ctx context.Context, userID, id int64) error {
	if id <= 0 {
		return domain.NewValidationError("notification", "Invalid notification.")
	}

	return s.repository.MarkNotificationRead(ctx, userID, id)
}

// MarkAllNotificationsRead marks every notification owned by a user as read.
func (s *Notifications) MarkAllNotificationsRead(ctx context.Context, userID int64) error {
	return s.repository.MarkAllNotificationsRead(ctx, userID)
}

// OpenNotification marks an owned notification read and returns its stored destination.
func (s *Notifications) OpenNotification(ctx context.Context, userID, id int64) (string, error) {
	if id <= 0 {
		return "", domain.NewValidationError("notification", "Invalid notification.")
	}

	return s.repository.OpenNotification(ctx, userID, id)
}
