package webhooks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	kit "github.com/containeroo/notifykit/notify"
	notifywebhook "github.com/containeroo/notifykit/targets/webhook"
	"github.com/containeroo/notifykit/templates"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

var webhookClient = notifywebhook.NewClient(5 * time.Second)

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
		s.recordWebhookDelivery(ctx, item.ID, event.Event, 0, 0, err.Error()) // nolint:errcheck
		return err
	}

	bodyTemplate, err := parseWebhookBodyTemplate(item.BodyTemplate)
	if err != nil {
		s.recordWebhookDelivery(ctx, item.ID, event.Event, 0, 0, err.Error()) // nolint:errcheck
		return err
	}
	titleTemplate, err := templates.ParseStringTemplate("kumbuka-webhook-title", `{{ .Input.Event }}`, templates.WithDefaultFuncs())
	if err != nil {
		s.recordWebhookDelivery(ctx, item.ID, event.Event, 0, 0, err.Error()) // nolint:errcheck
		return err
	}

	target := notifywebhook.New(
		notifywebhook.WithName(item.Name),
		notifywebhook.WithURL(item.URL),
		notifywebhook.WithHeaders(headers),
		notifywebhook.WithTitleTemplate(titleTemplate),
		notifywebhook.WithTemplate(bodyTemplate),
		notifywebhook.WithClient(webhookClient),
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
	recordErr := s.recordWebhookDelivery(ctx, item.ID, event.Event, recorder.result.StatusCode, recorder.attempts, message)
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

// recordWebhookDelivery persists one delivery outcome and reports history failures.
// Callers may preserve a more important primary delivery error while the log keeps
// the secondary persistence failure observable.
func (s *Webhooks) recordWebhookDelivery(ctx context.Context, webhookID int64, event string, statusCode, attempts int, message string) error {
	err := s.repository.AddWebhookDelivery(ctx, webhookID, event, statusCode, attempts, message)
	if err != nil {
		s.logger.ErrorContext(ctx,
			"record webhook delivery",
			"event", "webhook_delivery_record_failed",
			"webhook_id", webhookID,
			"webhook_event", event,
			"status_code", statusCode,
			"attempts", attempts,
			"error", err,
		)
	}

	return err
}

// webhookNotification adapts a Kumbuka event to Notifykit's Notification contract.
type webhookNotification struct {
	// event stores the event value used by webhook notification.
	event OutgoingEvent
	// publicURL is the externally visible base URL available to webhook templates.
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

// webhookRenderData groups data used by webhook render data.
type webhookRenderData struct {
	// Input stores the input value used by webhook render data.
	Input webhookTemplateInput
	// Payload stores the payload value used by webhook render data.
	Payload webhookTemplatePayload
	// Receiver stores the receiver value used by webhook render data.
	Receiver string
	// Title is the title associated with webhook render data.
	Title string
}

// webhookTemplateInput groups data used by webhook template input.
type webhookTemplateInput struct {
	// Event stores the event value used by webhook template input.
	Event string `json:"event"`
}

// webhookTemplatePayload groups data used by webhook template payload.
type webhookTemplatePayload struct {
	// ActorID identifies the actor associated with webhook template payload.
	ActorID int64 `json:"actor_id"`
	// ObjectType is the object type associated with webhook template payload.
	ObjectType string `json:"object_type"`
	// ObjectKey stores the object key value used by webhook template payload.
	ObjectKey string `json:"object_key"`
	// Detail stores the detail value used by webhook template payload.
	Detail string `json:"detail"`
	// OccurredAt records the occurred at timestamp for webhook template payload.
	OccurredAt time.Time `json:"occurred_at"`
	// URL is the target URL for webhook template payload.
	URL string `json:"url"`
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
	// target stores the target value used by webhook delivery target.
	target kit.Target
	// result stores the result value used by webhook delivery target.
	result kit.DeliveryResult
	// attempts stores the attempts value used by webhook delivery target.
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
