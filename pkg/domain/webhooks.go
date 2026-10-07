package domain

import "time"

// WebhookHeader is one configurable HTTP header sent with a webhook request.
type WebhookHeader struct {
	// ID identifies webhook header.
	ID int64
	// Name is the name of webhook header.
	Name string
	// Value contains the value represented by webhook header.
	Value string `json:"-"`
	// Sensitive marks a webhook header value as secret.
	Sensitive bool
	// Configured reports whether a sensitive header has a stored value without exposing it.
	Configured bool
}

// Webhook is one administrator-configured outgoing event destination.
type Webhook struct {
	// ID identifies webhook.
	ID int64
	// Name is the name of webhook.
	Name string
	// URL is the target URL for webhook.
	URL string
	// Events contains the events associated with webhook.
	Events []string
	// BodyTemplate formats the outgoing request body.
	BodyTemplate string
	// IncludeUserDetails makes actor and recipient contact fields available to the payload template.
	IncludeUserDetails bool
	// Headers contains the headers associated with webhook.
	Headers []WebhookHeader
	// RetryEnabled allows failed webhook deliveries to be retried.
	RetryEnabled bool
	// RetryCount is the number of retry associated with webhook.
	RetryCount int
	// RetryBackoff is the initial delay between delivery attempts.
	RetryBackoff time.Duration
	// RetryMaxBackoff caps the delay between delivery attempts.
	RetryMaxBackoff time.Duration
	// RetryJitter randomizes retry delays to avoid synchronized retries.
	RetryJitter bool
	// Enabled controls whether the webhook receives events.
	Enabled bool
	// CreatedAt records the created at timestamp for webhook.
	CreatedAt time.Time
	// UpdatedAt records the updated at timestamp for webhook.
	UpdatedAt time.Time
}

// WebhookDelivery records the latest outcome of delivering an outgoing event.
type WebhookDelivery struct {
	// ID identifies webhook delivery.
	ID int64
	// WebhookID identifies the webhook associated with webhook delivery.
	WebhookID int64
	// WebhookName is the webhook name associated with webhook delivery.
	WebhookName string
	// Event is the event name sent by this delivery.
	Event string
	// StatusCode is the HTTP response status returned by the receiver.
	StatusCode int
	// Attempts is the number of delivery attempts made.
	Attempts int
	// Error describes the final delivery failure.
	Error string
	// CreatedAt records the created at timestamp for webhook delivery.
	CreatedAt time.Time
}
