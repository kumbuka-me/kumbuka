package notifications

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithLoggerNormalizesNil verifies notification diagnostics always retain a usable logger.
func TestWithLoggerNormalizesNil(t *testing.T) {
	service := NewNotifications(nil).WithLogger(nil)

	assert.Same(t, slog.Default(), service.logger)
}

// notificationRepositoryStub provides controllable notification repository behavior for tests.
type notificationRepositoryStub struct {
	// limit records the limit observed by the test double.
	limit int
	// markedID records the ID passed to mark operations.
	markedID int64
	// unreadID records the unread ID observed by the test double.
	unreadID int64
	// markedAllID records the all ID passed to mark operations.
	markedAllID int64
	// deletedID records the ID passed to delete operations.
	deletedID int64
	// openedID records the ID passed to open operations.
	openedID int64
	// created records the plugin-created notification passed to persistence.
	created domain.Notification
	// createNew reports whether persistence inserted a new row.
	createNew bool
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

// User returns one enabled notification recipient.
func (*notificationRepositoryStub) User(_ context.Context, id int64) (domain.User, error) {
	return domain.User{ID: id, Username: "alice", DisplayName: "Alice", Enabled: true}, nil
}

// UserByUsername returns one enabled notification recipient by mention name.
func (*notificationRepositoryStub) UserByUsername(_ context.Context, username string) (domain.User, error) {
	return domain.User{ID: 42, Username: username, DisplayName: "Alice", Enabled: true}, nil
}

// CreateNotification records and returns one committed notification.
func (s *notificationRepositoryStub) CreateNotification(_ context.Context, item domain.Notification) (domain.Notification, bool, error) {
	s.created = item
	item.ID = 7
	item.CreatedAt = time.Date(2026, time.September, 25, 10, 30, 0, 0, time.UTC)
	return item, s.createNew, nil
}

// notificationEventSink records the most recent emitted event.
type notificationEventSink struct {
	event webhooks.OutgoingEvent
}

// Emit records one notification event.
func (s *notificationEventSink) Emit(_ context.Context, event webhooks.OutgoingEvent) error {
	s.event = event
	return nil
}

// TestSendPluginCreatesAttributedNotificationAndEvent verifies mutation attribution and webhook payloads.
func TestSendPluginCreatesAttributedNotificationAndEvent(t *testing.T) {
	t.Parallel()
	repository := &notificationRepositoryStub{createNew: true}
	sink := &notificationEventSink{}
	service := NewNotifications(repository, sink)

	receipt, err := service.SendPlugin(context.Background(), 9, "me.example.tasks", "Tasks", sdk.NotificationInput{
		RecipientUserID: 42,
		Title:           "Task assigned",
		Body:            "Review the plan.",
		URL:             "/pages/plan",
		IdempotencyKey:  "task:one:assigned",
	})

	require.NoError(t, err)
	assert.Equal(t, int64(7), receipt.ID)
	assert.Equal(t, domain.NotificationKindPlugin, repository.created.Kind)
	assert.Equal(t, int64(9), repository.created.ActorID)
	assert.Equal(t, "me.example.tasks", repository.created.SourceID)
	assert.Equal(t, "notification.created", sink.event.Event)
	assert.Equal(t, int64(9), sink.event.ActorID)
	assert.Equal(t, int64(42), sink.event.RecipientUserID)
	recipient := sink.event.Data["recipient"].(map[string]any)
	assert.Equal(t, "@alice", recipient["mention"])
}

// TestSendMentionsCreatesCoreNotificationAndEvent verifies page mentions include self-mentions and emit notification webhooks.
func TestSendMentionsCreatesCoreNotificationAndEvent(t *testing.T) {
	t.Parallel()
	repository := &notificationRepositoryStub{createNew: true}
	sink := &notificationEventSink{}

	err := NewNotifications(repository, sink).SendMentions(
		context.Background(),
		42,
		"Please review, @alice.",
		"Mention in Example",
		"/pages/example",
	)

	require.NoError(t, err)
	assert.Equal(t, domain.NotificationKindMention, repository.created.Kind)
	assert.Equal(t, domain.NotificationSourceCore, repository.created.SourceType)
	assert.Equal(t, int64(42), repository.created.ActorID)
	assert.Equal(t, int64(42), repository.created.RecipientUserID)
	assert.Equal(t, "notification.created", sink.event.Event)
	assert.Equal(t, int64(42), sink.event.ActorID)
	assert.Equal(t, int64(42), sink.event.RecipientUserID)
}

// TestSendPluginDoesNotEmitForIdempotentReplay verifies retried mutations do not duplicate events.
func TestSendPluginDoesNotEmitForIdempotentReplay(t *testing.T) {
	t.Parallel()
	repository := &notificationRepositoryStub{createNew: false}
	sink := &notificationEventSink{}
	_, err := NewNotifications(repository, sink).SendPlugin(context.Background(), 9, "me.example.tasks", "Tasks", sdk.NotificationInput{
		RecipientUserID: 42,
		Title:           "Task assigned",
		IdempotencyKey:  "task:one:assigned",
	})
	require.NoError(t, err)
	assert.Empty(t, sink.event.Event)
}

// TestSendPluginRejectsExternalDestinations verifies plugins cannot turn inbox links into arbitrary redirects.
func TestSendPluginRejectsExternalDestinations(t *testing.T) {
	t.Parallel()
	repository := &notificationRepositoryStub{createNew: true}
	_, err := NewNotifications(repository).SendPlugin(context.Background(), 9, "me.example.tasks", "Tasks", sdk.NotificationInput{
		RecipientUserID: 42,
		Title:           "Task assigned",
		URL:             "https://example.test/phishing",
		IdempotencyKey:  "task:one:assigned",
	})
	require.Error(t, err)
	assert.Empty(t, repository.created.Title)
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
