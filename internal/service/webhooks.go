package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	kit "github.com/containeroo/notifykit/notify"
	notifywebhook "github.com/containeroo/notifykit/targets/webhook"
	"github.com/containeroo/notifykit/templates"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"golang.org/x/net/http/httpguts"
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
	Event      string    `json:"event"`
	ActorID    int64     `json:"actor_id,omitempty"`
	ObjectType string    `json:"object_type"`
	ObjectKey  string    `json:"object_key"`
	Detail     string    `json:"detail,omitempty"`
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
	ID        int64
	Name      string
	Value     string
	Sensitive bool
}

// WebhookInput contains one administrator-supplied webhook configuration.
type WebhookInput struct {
	Name            string
	URL             string
	Events          []string
	BodyTemplate    string
	Headers         []WebhookHeaderInput
	RetryEnabled    bool
	RetryCount      int
	RetryBackoff    time.Duration
	RetryMaxBackoff time.Duration
	RetryJitter     bool
	Enabled         bool
}

// Webhooks owns persisted webhook configuration and Notifykit delivery.
type Webhooks struct {
	repository webhookRepository
	secrets    *secrets.Cipher
	client     *http.Client
	logger     *slog.Logger
	publicURL  string
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

// validateWebhookInput returns all user-correctable webhook configuration failures.
func validateWebhookInput(input WebhookInput, events []string) error {
	validation := &ValidationError{}

	if input.Name == "" {
		validation.Fields = append(validation.Fields, FieldError{Field: "name", Message: "A webhook name is required."})
	}

	parsed, err := url.ParseRequestURI(input.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		validation.Fields = append(validation.Fields, FieldError{Field: "url", Message: "Enter an absolute HTTP or HTTPS URL."})
	}

	if len(events) == 0 {
		validation.Fields = append(validation.Fields, FieldError{Field: "events", Message: "Choose at least one event."})
	}

	if err := validateWebhookBodyTemplate(input.BodyTemplate); err != nil {
		validation.Fields = append(validation.Fields, FieldError{
			Field:   "body_template",
			Message: "Payload template is invalid: " + err.Error(),
		})
	}

	if input.RetryEnabled {
		if input.RetryCount < 1 || input.RetryCount > maxWebhookRetryCount {
			validation.Fields = append(validation.Fields, FieldError{Field: "retry_count", Message: "Retries must be between 1 and 10."})
		}
		if input.RetryBackoff <= 0 || input.RetryBackoff > maxWebhookRetryBackoff {
			validation.Fields = append(validation.Fields, FieldError{Field: "retry_backoff", Message: "Initial backoff must be greater than zero and at most 1h."})
		}
		if input.RetryMaxBackoff < input.RetryBackoff || input.RetryMaxBackoff > maxWebhookRetryBackoff {
			validation.Fields = append(validation.Fields, FieldError{Field: "retry_max_backoff", Message: "Maximum backoff must be at least the initial backoff and at most 1h."})
		}
	}

	if len(validation.Fields) == 0 {
		return nil
	}

	return validation
}

// validateWebhookBodyTemplate parses and renders one template against the documented webhook context.
func validateWebhookBodyTemplate(value string) error {
	tmpl, err := parseWebhookBodyTemplate(value)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	event := OutgoingEvent{
		Event:      "page.updated",
		ActorID:    42,
		ObjectType: "page",
		ObjectKey:  "guides/example",
		Detail:     `Example "page" updated`,
		OccurredAt: time.Date(2026, 9, 11, 8, 0, 0, 0, time.UTC),
	}
	body, err := tmpl.Render(webhookNotification{event: event, publicURL: "https://kumbuka.example"}.Data("Example", nil, event.Event))
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}
	if !json.Valid(body) {
		return errors.New("result is not valid JSON; use | json for JSON values")
	}

	return nil
}

// prepareWebhookHeaders validates headers and encrypts changed sensitive values.
func (s *Webhooks) prepareWebhookHeaders(ctx context.Context, webhookID int64, inputs []WebhookHeaderInput) ([]domain.WebhookHeader, error) {
	existingByID := map[int64]domain.WebhookHeader{}
	if webhookID != 0 {
		existing, err := s.repository.Webhook(ctx, webhookID)
		if err != nil {
			return nil, err
		}
		for _, header := range existing.Headers {
			existingByID[header.ID] = header
		}
	}

	seenNames := make(map[string]struct{}, len(inputs))
	seenIDs := make(map[int64]struct{}, len(inputs))
	headers := make([]domain.WebhookHeader, 0, len(inputs))

	for _, input := range inputs {
		name, err := normalizeWebhookHeaderName(input.Name)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(name)
		if _, exists := seenNames[key]; exists {
			return nil, domain.NewValidationError("headers", "Webhook header names must be unique.")
		}
		seenNames[key] = struct{}{}

		previous, err := existingWebhookHeader(input.ID, existingByID, seenIDs)
		if err != nil {
			return nil, err
		}

		value := input.Value
		if strings.TrimSpace(value) == "" {
			if previous.ID != 0 && previous.Sensitive && input.Sensitive {
				value = previous.Value
			} else {
				return nil, domain.NewValidationError("headers", "Enter a value for every webhook request header.")
			}
		} else {
			if !httpguts.ValidHeaderFieldValue(value) {
				return nil, domain.NewValidationError("headers", "Webhook header values must be valid HTTP header values.")
			}
			if input.Sensitive {
				if s.secrets == nil || !s.secrets.Configured() {
					return nil, domain.NewValidationError("headers", "Configure KUMBUKA__ENCRYPTION_KEY before saving sensitive webhook headers.")
				}
				value, err = s.secrets.Encrypt(value)
				if err != nil {
					return nil, err
				}
			}
		}

		headers = append(headers, domain.WebhookHeader{
			ID:        previous.ID,
			Name:      name,
			Value:     value,
			Sensitive: input.Sensitive,
		})
	}

	return headers, nil
}

// existingWebhookHeader resolves a persisted header and rejects duplicate submitted identifiers.
func existingWebhookHeader(id int64, existing map[int64]domain.WebhookHeader, seen map[int64]struct{}) (domain.WebhookHeader, error) {
	if id == 0 {
		return domain.WebhookHeader{}, nil
	}
	if _, duplicate := seen[id]; duplicate {
		return domain.WebhookHeader{}, domain.NewValidationError("headers", "Webhook header rows must be unique.")
	}
	seen[id] = struct{}{}

	header, ok := existing[id]
	if !ok {
		return domain.WebhookHeader{}, domain.NewValidationError("headers", "One webhook header no longer exists. Reload the page and try again.")
	}
	return header, nil
}

// normalizeWebhookHeaderName validates a configurable HTTP header name.
func normalizeWebhookHeaderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", domain.NewValidationError("headers", "Enter a name for every webhook request header.")
	}
	if !httpguts.ValidHeaderFieldName(name) {
		return "", domain.NewValidationError("headers", "Webhook header names must be valid HTTP header names.")
	}

	canonicalName := http.CanonicalHeaderKey(name)
	if _, forbidden := reservedWebhookHeaderNames[canonicalName]; forbidden {
		return "", domain.NewValidationError("headers", canonicalName+" cannot be configured as a webhook request header.")
	}

	return canonicalName, nil
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

// TestWebhook sends a diagnostic event to one webhook regardless of its filters.
func (s *Webhooks) TestWebhook(ctx context.Context, id int64) error {
	item, err := s.repository.Webhook(ctx, id)
	if err != nil {
		return err
	}
	normalizeStoredWebhook(&item)

	event := OutgoingEvent{
		Event:      "webhook.test",
		ObjectType: "webhook",
		ObjectKey:  item.Name,
		Detail:     "Test delivery from Kumbuka",
		OccurredAt: time.Now().UTC(),
	}

	return s.deliver(ctx, item, event)
}

// Emit delivers one committed application event to every matching webhook.
func (s *Webhooks) Emit(ctx context.Context, event OutgoingEvent) error {
	items, err := s.repository.Webhooks(ctx)
	if err != nil {
		return err
	}

	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	var combined error

	for _, item := range items {
		if !item.Enabled || !slices.Contains(item.Events, event.Event) {
			continue
		}
		normalizeStoredWebhook(&item)

		if deliveryErr := s.deliver(ctx, item, event); deliveryErr != nil {
			combined = errors.Join(combined, deliveryErr)
		}
	}

	return combined
}

// deliver renders and sends one webhook through Notifykit and records its final outcome.
func (s *Webhooks) deliver(ctx context.Context, item domain.Webhook, event OutgoingEvent) error {
	headers, err := s.webhookRequestHeaders(item, event)
	if err != nil {
		s.recordWebhookDelivery(ctx, item.ID, event.Event, 0, 0, err.Error())
		return err
	}

	bodyTemplate, err := parseWebhookBodyTemplate(item.BodyTemplate)
	if err != nil {
		s.recordWebhookDelivery(ctx, item.ID, event.Event, 0, 0, err.Error())
		return err
	}
	titleTemplate, err := templates.ParseStringTemplate("kumbuka-webhook-title", `{{ .Input.Event }}`, templates.WithDefaultFuncs())
	if err != nil {
		s.recordWebhookDelivery(ctx, item.ID, event.Event, 0, 0, err.Error())
		return err
	}

	target := notifywebhook.New(
		notifywebhook.WithName(item.Name),
		notifywebhook.WithURL(item.URL),
		notifywebhook.WithHeaders(headers),
		notifywebhook.WithTitleTemplate(titleTemplate),
		notifywebhook.WithTemplate(bodyTemplate),
		notifywebhook.WithClient(s.client),
		notifywebhook.WithLogger(s.logger),
		notifywebhook.WithValidateJSON(),
		notifywebhook.WithLogResponse(notifywebhook.LogResponseNone),
	)

	recorder := &webhookDeliveryTarget{target: target}
	receiver := kit.NewReceiver(kit.ReceiverID(fmt.Sprintf("webhook-%d", item.ID)), recorder).WithName(item.Name)
	if item.RetryEnabled {
		receiver.WithRetry(kit.RetryConfig{
			Count:      item.RetryCount,
			Backoff:    item.RetryBackoff,
			MaxBackoff: item.RetryMaxBackoff,
			Jitter:     item.RetryJitter,
			Policy:     kit.DefaultRetryPolicy,
		})
	}

	notification := webhookNotification{event: event, publicURL: s.publicURL}
	deliveryErr := kit.Send(ctx, notification, kit.NewReceivers(receiver), s.logger)
	message := ""
	if deliveryErr != nil {
		message = deliveryErr.Error()
	}
	recordErr := s.repository.AddWebhookDelivery(ctx, item.ID, event.Event, recorder.result.StatusCode, recorder.attempts, message)
	if deliveryErr != nil {
		return deliveryErr
	}
	return recordErr
}

// webhookRequestHeaders decrypts sensitive headers and adds Kumbuka request metadata.
func (s *Webhooks) webhookRequestHeaders(item domain.Webhook, event OutgoingEvent) (map[string]string, error) {
	headers := map[string]string{
		"User-Agent":      "Kumbuka-Webhook/1",
		"X-Kumbuka-Event": event.Event,
	}

	for _, header := range item.Headers {
		value := header.Value
		if header.Sensitive {
			if s.secrets == nil || !s.secrets.Configured() {
				return nil, secrets.ErrNotConfigured
			}
			plain, err := s.secrets.Decrypt(value)
			if err != nil {
				return nil, err
			}
			value = plain
		}
		headers[header.Name] = value
	}

	return headers, nil
}

// recordWebhookDelivery records a failed delivery when the primary error must be preserved.
func (s *Webhooks) recordWebhookDelivery(ctx context.Context, webhookID int64, event string, statusCode, attempts int, message string) {
	_ = s.repository.AddWebhookDelivery(ctx, webhookID, event, statusCode, attempts, message)
}

// normalizeStoredWebhook applies defaults to rows created before configurable delivery settings existed.
func normalizeStoredWebhook(item *domain.Webhook) {
	if strings.TrimSpace(item.BodyTemplate) == "" {
		item.BodyTemplate = defaultWebhookBodyTemplate
	}
}

// maskedWebhookHeaders returns a redacted copy without mutating repository-owned values.
func maskedWebhookHeaders(headers []domain.WebhookHeader) []domain.WebhookHeader {
	masked := slices.Clone(headers)
	for index := range masked {
		if !masked[index].Sensitive {
			continue
		}
		masked[index].Configured = masked[index].Value != ""
		masked[index].Value = ""
	}
	return masked
}

// parseWebhookBodyTemplate parses one JSON body template with Notifykit's safe helper set.
func parseWebhookBodyTemplate(value string) (*templates.Template, error) {
	return templates.ParseTemplate("kumbuka-webhook-body", value, templates.WithDefaultFuncs())
}

// normalizeWebhookEvents filters, deduplicates, and preserves supported events.
func normalizeWebhookEvents(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}

	for _, value := range values {
		value = strings.TrimSpace(value)
		if !slices.Contains(webhookEvents, value) {
			continue
		}
		if seen[value] {
			continue
		}

		seen[value] = true
		result = append(result, value)
	}

	return result
}

// webhookNotification adapts a Kumbuka event to Notifykit's Notification contract.
type webhookNotification struct {
	event     OutgoingEvent
	publicURL string
}

// ID returns a stable-enough delivery identifier for structured Notifykit logs.
func (n webhookNotification) ID() string {
	return n.event.Event + ":" + n.event.ObjectKey
}

// Data exposes the stable template context used by administrator payload templates.
func (n webhookNotification) Data(receiver string, _ map[string]any, title string) any {
	return webhookRenderData{
		Input: webhookTemplateInput{Event: n.event.Event},
		Payload: webhookTemplatePayload{
			ActorID:    n.event.ActorID,
			ObjectType: n.event.ObjectType,
			ObjectKey:  n.event.ObjectKey,
			Detail:     n.event.Detail,
			OccurredAt: n.event.OccurredAt,
			URL:        webhookObjectURL(n.publicURL, n.event),
		},
		Receiver: receiver,
		Title:    title,
	}
}

type webhookRenderData struct {
	Input    webhookTemplateInput
	Payload  webhookTemplatePayload
	Receiver string
	Title    string
}

type webhookTemplateInput struct {
	Event string `json:"event"`
}

type webhookTemplatePayload struct {
	ActorID    int64     `json:"actor_id"`
	ObjectType string    `json:"object_type"`
	ObjectKey  string    `json:"object_key"`
	Detail     string    `json:"detail"`
	OccurredAt time.Time `json:"occurred_at"`
	URL        string    `json:"url"`
}

// webhookObjectURL returns a public page URL when the event identifies a page.
func webhookObjectURL(publicURL string, event OutgoingEvent) string {
	if publicURL == "" || event.ObjectType != "page" || event.ObjectKey == "" {
		return ""
	}
	return publicURL + "/pages/" + strings.TrimLeft(event.ObjectKey, "/")
}

// webhookDeliveryTarget captures the final Notifykit target result and attempt count.
type webhookDeliveryTarget struct {
	target   kit.Target
	result   kit.DeliveryResult
	attempts int
}

// Send forwards one attempt while remembering the latest delivery result.
func (t *webhookDeliveryTarget) Send(ctx context.Context, payload kit.Payload) (kit.DeliveryResult, error) {
	result, err := t.target.Send(ctx, payload)
	t.result = result
	t.attempts++
	return result, err
}

// Type reports the wrapped target type.
func (t *webhookDeliveryTarget) Type() string {
	return t.target.Type()
}
