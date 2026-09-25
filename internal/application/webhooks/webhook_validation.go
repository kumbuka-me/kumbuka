package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/containeroo/notifykit/templates"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"golang.org/x/net/http/httpguts"
)

// validateWebhookInput returns all user-correctable webhook configuration failures.
func validateWebhookInput(input WebhookInput, events []string) error {
	validation := &domain.ValidationError{}
	validateWebhookIdentity(input, events, validation)
	validateWebhookTemplate(input.BodyTemplate, validation)
	validateWebhookRetry(input, validation)
	if len(validation.Fields) == 0 {
		return nil
	}
	return validation
}

// validateWebhookIdentity validates the name, destination URL, and selected events.
func validateWebhookIdentity(input WebhookInput, events []string, validation *domain.ValidationError) {
	if input.Name == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "name",
			Message: "A webhook name is required.",
		})
	}
	parsed, err := url.ParseRequestURI(input.URL)
	if err != nil || !validWebhookURL(parsed) {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "url",
			Message: "Enter an absolute HTTP or HTTPS URL.",
		})
	}
	if len(events) == 0 {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "events",
			Message: "Choose at least one event.",
		})
	}
}

// validateWebhookTemplate validates the optional request body template.
func validateWebhookTemplate(value string, validation *domain.ValidationError) {
	if err := validateWebhookBodyTemplate(value); err != nil {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "body_template",
			Message: "Payload template is invalid: " + err.Error(),
		})
	}
}

// validateWebhookRetry validates retry count and backoff settings when retries are enabled.
func validateWebhookRetry(input WebhookInput, validation *domain.ValidationError) {
	if !input.RetryEnabled {
		return
	}
	if input.RetryCount < 1 || input.RetryCount > maxWebhookRetryCount {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "retry_count",
			Message: "Retries must be between 1 and 10.",
		})
	}
	if input.RetryBackoff <= 0 || input.RetryBackoff > maxWebhookRetryBackoff {
		validation.Fields = append(
			validation.Fields,
			domain.FieldError{
				Field: "retry_backoff", Message: "Initial backoff must be greater than zero and at most 1h.",
			},
		)
	}
	if !validWebhookMaximumBackoff(input) {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "retry_max_backoff",
			Message: "Maximum backoff must be at least the initial backoff and at most 1h.",
		})
	}
}

// validWebhookMaximumBackoff reports whether the maximum retry delay is ordered and within the configured cap.
func validWebhookMaximumBackoff(input WebhookInput) bool {
	return input.RetryMaxBackoff >= input.RetryBackoff && input.RetryMaxBackoff <= maxWebhookRetryBackoff
}

// validWebhookURL reports whether parsed is an absolute HTTP or HTTPS webhook destination.
func validWebhookURL(parsed *url.URL) bool {
	return parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

// preserveWebhookHeaderValue reports whether an unchanged sensitive header may keep its encrypted value.
func preserveWebhookHeaderValue(previous domain.WebhookHeader, input WebhookHeaderInput) bool {
	return previous.ID != 0 && previous.Sensitive && input.Sensitive
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
	sampleUser := &webhookTemplateUser{
		ID:          42,
		Mention:     "@alice",
		DisplayName: "Alice Example",
		Email:       "alice@example.test",
		Enabled:     true,
	}
	body, err := tmpl.Render(webhookNotification{
		event:     event,
		publicURL: "https://kumbuka.example",
		actor:     sampleUser,
		recipient: sampleUser,
	}.Data("Example", nil, event.Event))
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
	existing, err := s.webhookHeadersByID(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	seenNames := make(map[string]struct{}, len(inputs))
	seenIDs := make(map[int64]struct{}, len(inputs))
	headers := make([]domain.WebhookHeader, 0, len(inputs))
	for _, input := range inputs {
		header, err := s.prepareWebhookHeader(input, existing, seenNames, seenIDs)
		if err != nil {
			return nil, err
		}
		headers = append(headers, header)
	}
	return headers, nil
}

// webhookHeadersByID loads persisted headers for edit validation.
func (s *Webhooks) webhookHeadersByID(ctx context.Context, webhookID int64) (map[int64]domain.WebhookHeader, error) {
	result := map[int64]domain.WebhookHeader{}
	if webhookID == 0 {
		return result, nil
	}
	existing, err := s.repository.Webhook(ctx, webhookID)
	if err != nil {
		return nil, err
	}
	for _, header := range existing.Headers {
		result[header.ID] = header
	}
	return result, nil
}

// prepareWebhookHeader validates and encrypts one submitted request header.
func (s *Webhooks) prepareWebhookHeader(
	input WebhookHeaderInput,
	existing map[int64]domain.WebhookHeader,
	seenNames map[string]struct{},
	seenIDs map[int64]struct{},
) (domain.WebhookHeader, error) {
	name, err := normalizeWebhookHeaderName(input.Name)
	if err != nil {
		return domain.WebhookHeader{}, err
	}
	key := strings.ToLower(name)
	if _, duplicate := seenNames[key]; duplicate {
		return domain.WebhookHeader{}, domain.NewValidationError("headers", "Webhook header names must be unique.")
	}
	seenNames[key] = struct{}{}

	previous, err := existingWebhookHeader(input.ID, existing, seenIDs)
	if err != nil {
		return domain.WebhookHeader{}, err
	}
	value, err := s.prepareWebhookHeaderValue(previous, input)
	if err != nil {
		return domain.WebhookHeader{}, err
	}
	return domain.WebhookHeader{
		ID:        previous.ID,
		Name:      name,
		Value:     value,
		Sensitive: input.Sensitive,
	}, nil
}

// prepareWebhookHeaderValue preserves unchanged secrets or validates and encrypts a replacement value.
func (s *Webhooks) prepareWebhookHeaderValue(previous domain.WebhookHeader, input WebhookHeaderInput) (string, error) {
	value := input.Value
	if strings.TrimSpace(value) == "" {
		if preserveWebhookHeaderValue(previous, input) {
			return previous.Value, nil
		}
		return "", domain.NewValidationError("headers", "Enter a value for every webhook request header.")
	}
	if !httpguts.ValidHeaderFieldValue(value) {
		return "", domain.NewValidationError("headers", "Webhook header values must be valid HTTP header values.")
	}
	if !input.Sensitive {
		return value, nil
	}
	if s.secrets == nil || !s.secrets.Configured() {
		return "", domain.NewValidationError("headers", "Configure KUMBUKA__ENCRYPTION_KEY before saving sensitive webhook headers.")
	}
	return s.secrets.Encrypt(value)
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

	canonicalName := textproto.CanonicalMIMEHeaderKey(name)
	if _, forbidden := reservedWebhookHeaderNames[canonicalName]; forbidden {
		return "", domain.NewValidationError("headers", canonicalName+" cannot be configured as a webhook request header.")
	}

	return canonicalName, nil
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
