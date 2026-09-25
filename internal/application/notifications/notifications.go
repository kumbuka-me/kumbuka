package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/kumbuka-me/kumbuka/internal/application/audit"
	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/sdk"
)

const (
	defaultNotificationListSize = 30
	maxNotificationListSize     = 100
	maxNotificationTitleBytes   = 200
	maxNotificationBodyBytes    = 2000
	maxNotificationURLBytes     = 2048
	maxIdempotencyKeyBytes      = 200
)

// CreateInput contains trusted attribution and plugin-supplied notification content.
type CreateInput struct {
	// RecipientUserID identifies the enabled Kumbuka user receiving the notification.
	RecipientUserID int64
	// ActorID identifies the authenticated user whose mutation requested the notification.
	ActorID int64
	// SourceID is the host-supplied plugin identifier.
	SourceID string
	// SourceName is the host-supplied plugin display name.
	SourceName string
	// Title is the concise notification heading.
	Title string
	// Body is optional plain-text detail.
	Body string
	// URL is an optional local Kumbuka destination.
	URL string
	// IdempotencyKey deduplicates retried plugin mutations.
	IdempotencyKey string
}

// notificationRepository contains persistence operations for the notification inbox.
type notificationRepository interface {
	Notifications(context.Context, int64, int) (notifications []domain.Notification, unread int, err error)
	MarkNotificationRead(context.Context, int64, int64) error
	MarkNotificationUnread(context.Context, int64, int64) error
	MarkAllNotificationsRead(context.Context, int64) error
	DeleteNotification(context.Context, int64, int64) error
	OpenNotification(context.Context, int64, int64) (string, error)
}

// notificationCreator contains persistence required only for plugin-created notifications.
type notificationCreator interface {
	User(context.Context, int64) (domain.User, error)
	CreateNotification(context.Context, domain.Notification) (domain.Notification, bool, error)
}

// Notifications exposes per-user notification inbox use cases.
type Notifications struct {
	// repository provides the persistence operations required by notifications.
	repository notificationRepository
	// logger records failures from best-effort event delivery.
	logger *slog.Logger
	// eventSinks receive committed notification events.
	eventSinks []webhooks.EventSink
}

// NewNotifications constructs the notification inbox service.
func NewNotifications(repository notificationRepository, eventSinks ...webhooks.EventSink) *Notifications {
	return &Notifications{repository: repository, logger: audit.Logger(nil), eventSinks: eventSinks}
}

// WithLogger uses logger for best-effort notification event diagnostics.
func (s *Notifications) WithLogger(logger *slog.Logger) *Notifications {
	s.logger = audit.Logger(logger)
	return s
}

// Send validates, attributes, and persists one plugin-created in-app notification.
func (s *Notifications) Send(ctx context.Context, input CreateInput) (domain.Notification, error) {
	input = normalizeCreateInput(input)
	if err := validateCreateInput(input); err != nil {
		return domain.Notification{}, err
	}
	creator, ok := s.repository.(notificationCreator)
	if !ok {
		return domain.Notification{}, errors.New("notification creation unavailable")
	}
	recipient, err := creator.User(ctx, input.RecipientUserID)
	if err != nil {
		return domain.Notification{}, err
	}
	if !recipient.Enabled {
		return domain.Notification{}, domain.ErrNotFound
	}

	item, created, err := creator.CreateNotification(ctx, domain.Notification{
		Kind:            domain.NotificationKindPlugin,
		Title:           input.Title,
		Body:            input.Body,
		URL:             input.URL,
		RecipientUserID: recipient.ID,
		ActorID:         input.ActorID,
		SourceType:      domain.NotificationSourcePlugin,
		SourceID:        input.SourceID,
		SourceName:      input.SourceName,
		IdempotencyKey:  input.IdempotencyKey,
	})
	if err != nil {
		return domain.Notification{}, err
	}
	if created {
		s.emitCreated(ctx, item, recipient)
	}
	return item, nil
}

// SendPlugin creates one notification using host-supplied actor and plugin attribution.
func (s *Notifications) SendPlugin(
	ctx context.Context,
	actorID int64,
	sourceID, sourceName string,
	input sdk.NotificationInput,
) (sdk.Notification, error) {
	item, err := s.Send(ctx, CreateInput{
		RecipientUserID: input.RecipientUserID,
		ActorID:         actorID,
		SourceID:        sourceID,
		SourceName:      sourceName,
		Title:           input.Title,
		Body:            input.Body,
		URL:             input.URL,
		IdempotencyKey:  input.IdempotencyKey,
	})
	return sdk.Notification{ID: item.ID, RecipientUserID: item.RecipientUserID, CreatedAt: item.CreatedAt}, err
}

// normalizeCreateInput trims plugin-controlled scalar values before validation.
func normalizeCreateInput(input CreateInput) CreateInput {
	input.Title = strings.TrimSpace(input.Title)
	input.Body = strings.TrimSpace(input.Body)
	input.URL = strings.TrimSpace(input.URL)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	return input
}

// validateCreateInput validates bounded content and local notification destinations.
func validateCreateInput(input CreateInput) error {
	validation := &domain.ValidationError{}
	if input.RecipientUserID <= 0 {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "recipient_user_id", Message: "Choose a valid recipient."})
	}
	if input.ActorID <= 0 || input.SourceID == "" || input.SourceName == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "source", Message: "Notification attribution is unavailable."})
	}
	if input.Title == "" || len(input.Title) > maxNotificationTitleBytes || !utf8.ValidString(input.Title) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "title", Message: "Use a valid notification title."})
	}
	if len(input.Body) > maxNotificationBodyBytes || !utf8.ValidString(input.Body) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "body", Message: "Notification body is too long."})
	}
	if input.IdempotencyKey == "" || len(input.IdempotencyKey) > maxIdempotencyKeyBytes || !utf8.ValidString(input.IdempotencyKey) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "idempotency_key", Message: "Use a valid idempotency key."})
	}
	if !validNotificationURL(input.URL) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "url", Message: "Use a local Kumbuka URL."})
	}
	if len(validation.Fields) != 0 {
		return validation
	}
	return nil
}

// validNotificationURL reports whether value is empty or a bounded local absolute path.
func validNotificationURL(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > maxNotificationURLBytes || !utf8.ValidString(value) {
		return false
	}
	parsed, err := url.Parse(value)
	return err == nil && !parsed.IsAbs() && parsed.Host == "" && strings.HasPrefix(parsed.Path, "/") && !strings.HasPrefix(value, "//")
}

// emitCreated delivers one committed notification event as a best-effort side effect.
func (s *Notifications) emitCreated(ctx context.Context, item domain.Notification, recipient domain.User) {
	event := webhooks.OutgoingEvent{
		Event:      webhooks.EventNotificationCreated,
		ActorID:    item.ActorID,
		ObjectType: "notification",
		ObjectKey:  fmt.Sprint(item.ID),
		OccurredAt: item.CreatedAt,
		Data: map[string]any{
			"recipient":    map[string]any{"user_id": recipient.ID, "mention": "@" + recipient.Username, "display_name": recipient.DisplayName},
			"notification": map[string]any{"title": item.Title, "body": item.Body, "url": item.URL, "source_type": item.SourceType, "source_id": item.SourceID, "source_name": item.SourceName},
		},
	}
	for _, sink := range s.eventSinks {
		if err := sink.Emit(ctx, event); err != nil {
			s.logger.ErrorContext(ctx, "notification event delivery failed", "event", "notification_event_failed", "notification_id", item.ID, "error", err)
		}
	}
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

// MarkNotificationUnread marks one owned notification as unread.
func (s *Notifications) MarkNotificationUnread(ctx context.Context, userID, id int64) error {
	if id <= 0 {
		return domain.NewValidationError("notification", "Invalid notification.")
	}

	return s.repository.MarkNotificationUnread(ctx, userID, id)
}

// MarkAllNotificationsRead marks every notification owned by a user as read.
func (s *Notifications) MarkAllNotificationsRead(ctx context.Context, userID int64) error {
	return s.repository.MarkAllNotificationsRead(ctx, userID)
}

// DeleteNotification removes one owned notification.
func (s *Notifications) DeleteNotification(ctx context.Context, userID, id int64) error {
	if id <= 0 {
		return domain.NewValidationError("notification", "Invalid notification.")
	}

	return s.repository.DeleteNotification(ctx, userID, id)
}

// OpenNotification marks an owned notification read and returns its stored destination.
func (s *Notifications) OpenNotification(ctx context.Context, userID, id int64) (string, error) {
	if id <= 0 {
		return "", domain.NewValidationError("notification", "Invalid notification.")
	}

	return s.repository.OpenNotification(ctx, userID, id)
}
