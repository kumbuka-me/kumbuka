package service

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	notifywebhook "github.com/containeroo/notifykit/targets/webhook"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const defaultWebhookBodyTemplate = `{
  "event": {{ .Input.Event | json }},
  "actor_id": {{ .Payload.ActorID | json }},
  "object_type": {{ .Payload.ObjectType | json }},
  "object_key": {{ .Payload.ObjectKey | json }},
  "detail": {{ .Payload.Detail | json }},
  "occurred_at": {{ .Payload.OccurredAt | json }},
  "url": {{ .Payload.URL | json }}
}`

const (
	defaultWebhookRetryCount      = 2
	defaultWebhookRetryBackoff    = time.Second
	defaultWebhookRetryMaxBackoff = 30 * time.Second
	maxWebhookRetryCount          = 10
	maxWebhookRetryBackoff        = time.Hour
)

var webhookEvents = []string{
	"page.created", "page.updated", "page.renamed", "page.deleted", "page.moved",
	"page.reviewed", "page.review_requested", "page.review_updated", "page.review_canceled",
	"page.review_approved", "page.review_changes_requested",
	"page.revision_restored", "comment.created", "pages.imported",
	"page.bulk_status", "page.bulk_tag", "page.bulk_group", "page.bulk_move", "page.bulk_delete",
}

var reservedWebhookHeaderNames = map[string]struct{}{
	"Connection":        {},
	"Content-Length":    {},
	"Content-Type":      {},
	"Host":              {},
	"Keep-Alive":        {},
	"Proxy-Connection":  {},
	"Te":                {},
	"Trailer":           {},
	"Transfer-Encoding": {},
	"Upgrade":           {},
}

// OutgoingEvent is the stable event emitted by application mutations.
type OutgoingEvent struct {
	// Event names the application event, for example page.updated.
	Event string `json:"event"`
	// ActorID identifies the user that caused the event when one exists.
	ActorID int64 `json:"actor_id,omitempty"`
	// ObjectType names the affected resource type.
	ObjectType string `json:"object_type"`
	// ObjectKey identifies the affected resource within its type.
	ObjectKey string `json:"object_key"`
	// Detail contains optional human-readable context for the event.
	Detail string `json:"detail,omitempty"`
	// OccurredAt records when the mutation committed.
	OccurredAt time.Time `json:"occurred_at"`
}

// EventSink receives committed application events as best-effort side effects.
type EventSink interface {
	Emit(context.Context, OutgoingEvent) error
}

type webhookRepository interface {
	Webhooks(context.Context) ([]domain.Webhook, error)
	Webhook(context.Context, int64) (domain.Webhook, error)
	SaveWebhook(context.Context, int64, domain.Webhook) (domain.Webhook, error)
	DeleteWebhook(context.Context, int64) error
	AddWebhookDelivery(context.Context, int64, string, int, int, string) error
	WebhookDeliveries(context.Context, int) ([]domain.WebhookDelivery, error)
}

// WebhookHeaderInput contains one administrator-supplied webhook request header.
type WebhookHeaderInput struct {
	// ID identifies an existing persisted header; zero creates a new row.
	ID int64
	// Name is the HTTP request header name.
	Name string
	// Value is the submitted plaintext value; an empty sensitive value preserves the stored secret.
	Value string
	// Sensitive encrypts the value at rest and redacts it from normal reads.
	Sensitive bool
}

// WebhookInput contains one administrator-supplied webhook configuration.
type WebhookInput struct {
	// Name is the administrator-visible webhook name.
	Name string
	// URL is the absolute HTTP or HTTPS delivery endpoint.
	URL string
	// Events selects the application events that trigger delivery.
	Events []string
	// BodyTemplate renders the JSON request body.
	BodyTemplate string
	// Headers contains additional administrator-configured request headers.
	Headers []WebhookHeaderInput
	// RetryEnabled enables retrying transient delivery failures.
	RetryEnabled bool
	// RetryCount is the maximum number of retry attempts after the first delivery.
	RetryCount int
	// RetryBackoff is the initial delay between retry attempts.
	RetryBackoff time.Duration
	// RetryMaxBackoff caps exponential retry delay.
	RetryMaxBackoff time.Duration
	// RetryJitter randomizes retry delays to avoid synchronized retries.
	RetryJitter bool
	// Enabled controls whether matching events are delivered.
	Enabled bool
}

// Webhooks owns persisted webhook configuration and Notifykit delivery.
type Webhooks struct {
	// repository persists webhook configuration and delivery history.
	repository webhookRepository
	// secrets encrypts sensitive request headers at rest.
	secrets *secrets.Cipher
	// client performs outbound webhook HTTP requests.
	client *http.Client
	// logger records diagnostics emitted by webhooks.
	logger *slog.Logger
	// publicURL is the externally visible base URL used when building webhook payloads.
	publicURL string
}

// NewWebhooks constructs outgoing webhook use cases.
func NewWebhooks(repository webhookRepository, secretCipher *secrets.Cipher, logger *slog.Logger, publicURL string) *Webhooks {
	return &Webhooks{
		repository: repository,
		secrets:    secretCipher,
		client:     notifywebhook.NewClient(5 * time.Second),
		logger:     logger,
		publicURL:  strings.TrimRight(strings.TrimSpace(publicURL), "/"),
	}
}

// DefaultWebhook returns the initial values used for a new webhook form.
func DefaultWebhook() domain.Webhook {
	return domain.Webhook{
		BodyTemplate:    defaultWebhookBodyTemplate,
		RetryEnabled:    false,
		RetryCount:      defaultWebhookRetryCount,
		RetryBackoff:    defaultWebhookRetryBackoff,
		RetryMaxBackoff: defaultWebhookRetryMaxBackoff,
		RetryJitter:     true,
		Enabled:         true,
	}
}

// WebhookEvents returns the supported outgoing event names.
func WebhookEvents() []string {
	return slices.Clone(webhookEvents)
}

// Webhooks returns configured webhooks with sensitive request headers redacted.
func (s *Webhooks) Webhooks(ctx context.Context) ([]domain.Webhook, error) {
	items, err := s.repository.Webhooks(ctx)
	if err != nil {
		return nil, err
	}
	for index := range items {
		normalizeStoredWebhook(&items[index])
		items[index].Headers = maskedWebhookHeaders(items[index].Headers)
	}
	return items, nil
}

// WebhookDeliveries returns recent outgoing delivery outcomes.
func (s *Webhooks) WebhookDeliveries(ctx context.Context, limit int) ([]domain.WebhookDelivery, error) {
	return s.repository.WebhookDeliveries(ctx, limit)
}

// SaveWebhook validates, protects, and persists one webhook configuration.
func (s *Webhooks) SaveWebhook(ctx context.Context, id int64, input WebhookInput) (domain.Webhook, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.URL = strings.TrimSpace(input.URL)
	input.BodyTemplate = strings.TrimSpace(input.BodyTemplate)
	if input.BodyTemplate == "" {
		input.BodyTemplate = defaultWebhookBodyTemplate
	}
	events := normalizeWebhookEvents(input.Events)

	if err := validateWebhookInput(input, events); err != nil {
		return domain.Webhook{}, err
	}

	headers, err := s.prepareWebhookHeaders(ctx, id, input.Headers)
	if err != nil {
		return domain.Webhook{}, err
	}

	item, err := s.repository.SaveWebhook(ctx, id, domain.Webhook{
		Name:            input.Name,
		URL:             input.URL,
		Events:          events,
		BodyTemplate:    input.BodyTemplate,
		Headers:         headers,
		RetryEnabled:    input.RetryEnabled,
		RetryCount:      input.RetryCount,
		RetryBackoff:    input.RetryBackoff,
		RetryMaxBackoff: input.RetryMaxBackoff,
		RetryJitter:     input.RetryJitter,
		Enabled:         input.Enabled,
	})
	if err != nil {
		return domain.Webhook{}, err
	}

	normalizeStoredWebhook(&item)
	item.Headers = maskedWebhookHeaders(item.Headers)

	return item, nil
}

// RevealWebhookHeader returns one persisted header value after an explicit administrator action.
func (s *Webhooks) RevealWebhookHeader(ctx context.Context, webhookID, headerID int64) (string, error) {
	item, err := s.repository.Webhook(ctx, webhookID)
	if err != nil {
		return "", err
	}

	for _, header := range item.Headers {
		if header.ID != headerID {
			continue
		}
		if !header.Sensitive {
			return header.Value, nil
		}
		if s.secrets == nil || !s.secrets.Configured() {
			return "", domain.NewValidationError("headers", "Configure KUMBUKA__ENCRYPTION_KEY before revealing sensitive webhook headers.")
		}
		return s.secrets.Decrypt(header.Value)
	}

	return "", domain.ErrNotFound
}

// DeleteWebhook removes one outgoing webhook configuration.
func (s *Webhooks) DeleteWebhook(ctx context.Context, id int64) error {
	return s.repository.DeleteWebhook(ctx, id)
}
