package notifications

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type notificationRepositoryStub struct {
	limit       int
	markedID    int64
	unreadID    int64
	markedAllID int64
	deletedID   int64
	openedID    int64
}

// Notifications records the requested limit and returns an empty inbox.
func (s *notificationRepositoryStub) Notifications(_ context.Context, _ int64, limit int) ([]domain.Notification, int, error) {
	s.limit = limit

	return nil, 0, nil
}

// MarkNotificationRead records the notification selected by the service.
func (s *notificationRepositoryStub) MarkNotificationRead(_ context.Context, _ int64, id int64) error {
	s.markedID = id

	return nil
}

// MarkNotificationUnread records the notification selected by the service.
func (s *notificationRepositoryStub) MarkNotificationUnread(_ context.Context, _ int64, id int64) error {
	s.unreadID = id

	return nil
}

// MarkAllNotificationsRead records the user selected by the service.
func (s *notificationRepositoryStub) MarkAllNotificationsRead(_ context.Context, userID int64) error {
	s.markedAllID = userID

	return nil
}

// DeleteNotification records the notification selected by the service.
func (s *notificationRepositoryStub) DeleteNotification(_ context.Context, _ int64, id int64) error {
	s.deletedID = id

	return nil
}

// OpenNotification records the notification selected by the service.
func (s *notificationRepositoryStub) OpenNotification(_ context.Context, _ int64, id int64) (string, error) {
	s.openedID = id

	return "/pages/example", nil
}

func TestNotificationsLimit(t *testing.T) {
	t.Parallel()

	t.Run("keeps requested limit", func(t *testing.T) {
		t.Parallel()

		repository := &notificationRepositoryStub{}
		_, _, err := NewNotifications(repository).Notifications(context.Background(), 7, 30)

		require.NoError(t, err)
		assert.Equal(t, 30, repository.limit)
	})

	t.Run("bounds zero limit", func(t *testing.T) {
		t.Parallel()

		repository := &notificationRepositoryStub{}
		_, _, err := NewNotifications(repository).Notifications(context.Background(), 7, 0)

		require.NoError(t, err)
		assert.Equal(t, defaultNotificationListSize, repository.limit)
	})

	t.Run("bounds oversized limit", func(t *testing.T) {
		t.Parallel()

		repository := &notificationRepositoryStub{}
		_, _, err := NewNotifications(repository).Notifications(context.Background(), 7, maxNotificationListSize+1)

		require.NoError(t, err)
		assert.Equal(t, maxNotificationListSize, repository.limit)
	})
}

func TestNotificationMutationsDelegateExplicitly(t *testing.T) {
	t.Parallel()

	t.Run("marks one", func(t *testing.T) {
		t.Parallel()

		repository := &notificationRepositoryStub{}
		require.NoError(t, NewNotifications(repository).MarkNotificationRead(context.Background(), 42, 7))

		assert.Equal(t, int64(7), repository.markedID)
		assert.Zero(t, repository.markedAllID)
	})

	t.Run("marks unread", func(t *testing.T) {
		t.Parallel()

		repository := &notificationRepositoryStub{}
		require.NoError(t, NewNotifications(repository).MarkNotificationUnread(context.Background(), 42, 8))

		assert.Equal(t, int64(8), repository.unreadID)
		assert.Zero(t, repository.markedID)
	})

	t.Run("marks all", func(t *testing.T) {
		t.Parallel()

		repository := &notificationRepositoryStub{}
		require.NoError(t, NewNotifications(repository).MarkAllNotificationsRead(context.Background(), 42))

		assert.Equal(t, int64(42), repository.markedAllID)
		assert.Zero(t, repository.markedID)
	})

	t.Run("deletes one", func(t *testing.T) {
		t.Parallel()

		repository := &notificationRepositoryStub{}
		require.NoError(t, NewNotifications(repository).DeleteNotification(context.Background(), 42, 9))

		assert.Equal(t, int64(9), repository.deletedID)
	})

	t.Run("opens one", func(t *testing.T) {
		t.Parallel()

		repository := &notificationRepositoryStub{}
		destination, err := NewNotifications(repository).OpenNotification(context.Background(), 42, 7)

		require.NoError(t, err)
		assert.Equal(t, "/pages/example", destination)
		assert.Equal(t, int64(7), repository.openedID)
	})
}
