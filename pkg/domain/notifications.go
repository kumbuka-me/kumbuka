package domain

import "time"

// Notification is a lightweight user inbox item.
type Notification struct {
	// ID identifies notification.
	ID int64 `json:"id"`
	// Kind identifies the notification category.
	Kind NotificationKind `json:"kind"`
	// Title is the title associated with notification.
	Title string `json:"title"`
	// Body contains the notification message.
	Body string `json:"body"`
	// URL is the target URL for notification.
	URL string `json:"url"`
	// RecipientUserID identifies the Kumbuka user who owns the notification.
	RecipientUserID int64 `json:"recipient_user_id,omitempty"`
	// ActorID identifies the user whose action created the notification when one exists.
	ActorID int64 `json:"actor_id,omitempty"`
	// SourceType identifies whether core or a plugin created the notification.
	SourceType NotificationSourceType `json:"source_type,omitempty"`
	// SourceID identifies the producing plugin, or is empty for core notifications.
	SourceID string `json:"source_id,omitempty"`
	// SourceName is the display-name snapshot of the notification producer.
	SourceName string `json:"source_name,omitempty"`
	// IdempotencyKey deduplicates plugin retries and is never exposed to inbox clients.
	IdempotencyKey string `json:"-"`
	// ReadAt records the read at timestamp for notification.
	ReadAt *time.Time `json:"read_at,omitempty"`
	// CreatedAt records the created at timestamp for notification.
	CreatedAt time.Time `json:"created_at"`
}
