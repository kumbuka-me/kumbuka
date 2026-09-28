package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/mention"
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

// notificationRepository contains all persistence required by notification inbox and delivery use cases.
type notificationRepository interface {
	Notifications(context.Context, int64, int) (notifications []domain.Notification, unread int, err error)
	MarkNotificationRead(context.Context, int64, int64) error
	MarkNotificationUnread(context.Context, int64, int64) error
	MarkAllNotificationsRead(context.Context, int64) error
	DeleteNotification(context.Context, int64, int64) error
	OpenNotification(context.Context, int64, int64) (string, error)
	User(context.Context, int64) (domain.User, error)
	UserByUsername(context.Context, string) (domain.User, error)
	CreateNotification(context.Context, domain.Notification) (domain.Notification, bool, error)
	ClaimPluginUpdateAnnouncements(context.Context, []domain.PluginUpdateNotice) ([]domain.PluginUpdateNotice, error)
	EnabledAdministratorIDs(context.Context) ([]int64, error)
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
func NewNotifications(
	repository notificationRepository,
	logger *slog.Logger,
	eventSinks ...webhooks.EventSink,
) *Notifications {
	return &Notifications{
		repository: repository,
		logger:     logger,
		eventSinks: eventSinks,
	}
}

// Send validates, attributes, and persists one plugin-created in-app notification.
func (s *Notifications) Send(ctx context.Context, input CreateInput) (domain.Notification, error) {
	input = normalizeCreateInput(input)
	if err := validateCreateInput(input); err != nil {
		return domain.Notification{}, err
	}
	recipient, err := s.repository.User(ctx, input.RecipientUserID)
	if err != nil {
		return domain.Notification{}, err
	}
	if !recipient.Enabled {
		return domain.Notification{}, domain.ErrNotFound
	}

	item, created, err := s.repository.CreateNotification(ctx, domain.Notification{
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

// SendCore creates one core-owned notification for an enabled recipient.
func (s *Notifications) SendCore(
	ctx context.Context,
	recipientUserID, actorID int64,
	kind domain.NotificationKind,
	title, body, destination string,
) error {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	destination = strings.TrimSpace(destination)
	if err := validateCoreDelivery(recipientUserID, actorID, kind, title, body, destination); err != nil {
		return err
	}
	recipient, err := s.repository.User(ctx, recipientUserID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !recipient.Enabled {
		return nil
	}

	return s.createCore(ctx, s.repository, recipient, actorID, kind, title, body, destination)
}

// SendMentions creates core-owned notifications for each distinct enabled mention, including self-mentions.
func (s *Notifications) SendMentions(ctx context.Context, actorID int64, text, title, destination string) error {
	title = strings.TrimSpace(title)
	destination = strings.TrimSpace(destination)
	body := "You were mentioned in page content."
	if err := validateCoreNotification(actorID, title, body, destination); err != nil {
		return err
	}
	for _, username := range mention.Usernames(text) {
		recipient, err := s.repository.UserByUsername(ctx, username)
		if errors.Is(err, domain.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		if !recipient.Enabled {
			continue
		}
		if err := s.createCore(
			ctx,
			s.repository,
			recipient,
			actorID,
			domain.NotificationKindMention,
			title,
			body,
			destination,
		); err != nil {
			return err
		}
	}

	return nil
}

// NotifyPluginUpdates creates one deduplicated core notification for each enabled administrator.
func (s *Notifications) NotifyPluginUpdates(ctx context.Context, updates []domain.PluginUpdateNotice) error {
	administratorIDs, err := s.repository.EnabledAdministratorIDs(ctx)
	if err != nil || len(administratorIDs) == 0 {
		return err
	}
	pending, err := s.repository.ClaimPluginUpdateAnnouncements(ctx, updates)
	if err != nil || len(pending) == 0 {
		return err
	}

	sort.Slice(pending, func(i, j int) bool {
		return strings.ToLower(pending[i].Name) < strings.ToLower(pending[j].Name)
	})
	title, body := pluginUpdateNotification(pending)
	for _, userID := range administratorIDs {
		if err := s.SendCore(ctx, userID, 0, domain.NotificationKindPluginUpdate, title, body, "/admin/plugins"); err != nil {
			return err
		}
	}
	return nil
}

// createCore persists and emits one core-owned notification for a resolved recipient.
func (s *Notifications) createCore(
	ctx context.Context,
	creator interface {
		CreateNotification(context.Context, domain.Notification) (domain.Notification, bool, error)
	},
	recipient domain.User,
	actorID int64,
	kind domain.NotificationKind,
	title, body, destination string,
) error {
	item, created, err := creator.CreateNotification(ctx, domain.Notification{
		Kind:            kind,
		Title:           title,
		Body:            body,
		URL:             destination,
		RecipientUserID: recipient.ID,
		ActorID:         actorID,
		SourceType:      domain.NotificationSourceCore,
		SourceName:      "Kumbuka",
	})
	if err != nil {
		return err
	}
	if created {
		s.emitCreated(ctx, item, recipient)
	}
	return nil
}

// pluginUpdateNotification formats one compact administrator notification for newly announced releases.
func pluginUpdateNotification(updates []domain.PluginUpdateNotice) (string, string) {
	if len(updates) == 1 {
		update := updates[0]
		return "Plugin update available", fmt.Sprintf(
			"%s %s is available; currently %s.",
			update.Name,
			update.AvailableVersion,
			update.CurrentVersion,
		)
	}

	entries := make([]string, 0, len(updates))
	for _, update := range updates {
		entries = append(entries, fmt.Sprintf(
			"%s %s -> %s",
			update.Name,
			update.CurrentVersion,
			update.AvailableVersion,
		))
	}
	return fmt.Sprintf("%d plugin updates available", len(updates)), strings.Join(entries, "; ")
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
	if !validNotificationTitle(input.Title) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "title", Message: "Use a valid notification title."})
	}
	if !validNotificationBody(input.Body) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "body", Message: "Notification body is too long."})
	}
	if !validIdempotencyKey(input.IdempotencyKey) {
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

// validateCoreNotification validates one trusted core notification before persistence.
func validateCoreNotification(actorID int64, title, body, url string) error {
	validation := &domain.ValidationError{}
	if actorID <= 0 {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "actor_id", Message: "Notification attribution is unavailable."})
	}
	if !validNotificationTitle(title) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "title", Message: "Use a valid notification title."})
	}
	if !validNotificationBody(body) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "body", Message: "Notification body is too long."})
	}
	if !validNotificationURL(url) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "url", Message: "Use a local Kumbuka URL."})
	}
	if len(validation.Fields) != 0 {
		return validation
	}
	return nil
}

// validateCoreDelivery validates one generic core-owned notification. Actor zero is reserved for system-generated notifications.
func validateCoreDelivery(recipientUserID, actorID int64, kind domain.NotificationKind, title, body, destination string) error {
	validation := &domain.ValidationError{}
	if recipientUserID <= 0 {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "recipient_user_id", Message: "Choose a valid recipient."})
	}
	if actorID < 0 {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "actor_id", Message: "Notification attribution is unavailable."})
	}
	if kind == "" || kind == domain.NotificationKindPlugin {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "kind", Message: "Choose a valid core notification kind."})
	}
	if !validNotificationTitle(title) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "title", Message: "Use a valid notification title."})
	}
	if !validNotificationBody(body) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "body", Message: "Notification body is too long."})
	}
	if !validNotificationURL(destination) {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "url", Message: "Use a local Kumbuka URL."})
	}
	if len(validation.Fields) != 0 {
		return validation
	}
	return nil
}

// validNotificationTitle reports whether title is non-empty, bounded, and valid UTF-8.
func validNotificationTitle(title string) bool {
	return title != "" && len(title) <= maxNotificationTitleBytes && utf8.ValidString(title)
}

// validNotificationBody reports whether body is bounded and valid UTF-8.
func validNotificationBody(body string) bool {
	return len(body) <= maxNotificationBodyBytes && utf8.ValidString(body)
}

// validIdempotencyKey reports whether key is non-empty, bounded, and valid UTF-8.
func validIdempotencyKey(key string) bool {
	return key != "" && len(key) <= maxIdempotencyKeyBytes && utf8.ValidString(key)
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
		Event:           webhooks.EventNotificationCreated,
		ActorID:         item.ActorID,
		RecipientUserID: recipient.ID,
		ObjectType:      "notification",
		ObjectKey:       fmt.Sprint(item.ID),
		OccurredAt:      item.CreatedAt,
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
