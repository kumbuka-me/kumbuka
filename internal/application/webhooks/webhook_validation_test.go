package webhooks

import (
	"errors"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWebhookInputCollectsIndependentFailures(t *testing.T) {
	t.Parallel()

	err := validateWebhookInput(WebhookInput{
		URL:             "not-a-url",
		BodyTemplate:    `{"value": {{`,
		RetryEnabled:    true,
		RetryCount:      0,
		RetryBackoff:    0,
		RetryMaxBackoff: 2 * time.Hour,
	}, nil)

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	fields := make(map[string]bool, len(validation.Fields))
	for _, field := range validation.Fields {
		fields[field.Field] = true
	}
	assert.True(t, fields["name"])
	assert.True(t, fields["url"])
	assert.True(t, fields["events"])
	assert.True(t, fields["body_template"])
	assert.True(t, fields["retry_count"])
	assert.True(t, fields["retry_backoff"])
	assert.True(t, fields["retry_max_backoff"])
}

func TestValidateWebhookInputIgnoresRetryValuesWhenDisabled(t *testing.T) {
	t.Parallel()

	err := validateWebhookInput(WebhookInput{
		Name:            "Deploy",
		URL:             "https://example.test/hook",
		BodyTemplate:    `{}`,
		RetryCount:      -1,
		RetryBackoff:    -time.Second,
		RetryMaxBackoff: -time.Second,
	}, []string{"page.updated"})

	require.NoError(t, err)
}

func TestValidateWebhookInputUsesSelectedEventShapes(t *testing.T) {
	t.Parallel()

	template := `{{- $notification := index .Payload.Data "notification" -}}
{
  "recipient": {{ .Payload.Recipient.Email | json }},
  "subject": {{ index $notification "title" | json }},
  "body": {{ index $notification "body" | json }}
}`

	t.Run("accepts notification-specific fields when notification is selected", func(t *testing.T) {
		t.Parallel()

		err := validateWebhookInput(WebhookInput{
			Name:               "mail",
			URL:                "https://example.test/mail",
			BodyTemplate:       template,
			IncludeUserDetails: true,
		}, []string{EventNotificationCreated})

		require.NoError(t, err)
	})

	t.Run("rejects notification-specific fields for an incompatible selected event", func(t *testing.T) {
		t.Parallel()

		err := validateWebhookInput(WebhookInput{
			Name:               "mail",
			URL:                "https://example.test/mail",
			BodyTemplate:       template,
			IncludeUserDetails: true,
		}, []string{"page.updated"})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "body_template", validation.Fields[0].Field)
		assert.Contains(t, validation.Fields[0].Message, "page.updated")
	})

	t.Run("requires the template to render for every selected event", func(t *testing.T) {
		t.Parallel()

		err := validateWebhookInput(WebhookInput{
			Name:               "mail",
			URL:                "https://example.test/mail",
			BodyTemplate:       template,
			IncludeUserDetails: true,
		}, []string{EventNotificationCreated, "page.updated"})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "body_template", validation.Fields[0].Field)
		assert.Contains(t, validation.Fields[0].Message, "page.updated")
	})
}

func TestValidateWebhookInputRespectsUserDetailsOption(t *testing.T) {
	t.Parallel()

	template := `{"recipient": {{ .Payload.Recipient.Email | json }}}`
	input := WebhookInput{
		Name:         "mail",
		URL:          "https://example.test/mail",
		BodyTemplate: template,
	}

	err := validateWebhookInput(input, []string{EventNotificationCreated})
	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "body_template", validation.Fields[0].Field)

	input.IncludeUserDetails = true
	require.NoError(t, validateWebhookInput(input, []string{EventNotificationCreated}))
}

func TestValidateWebhookInputAcceptsDefaultTemplateForAllSupportedEvents(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateWebhookInput(WebhookInput{
		Name:         "all-events",
		URL:          "https://example.test/hook",
		BodyTemplate: defaultWebhookBodyTemplate,
	}, WebhookEvents()))
}

func TestValidateWebhookInputProvidesAllNotificationFields(t *testing.T) {
	t.Parallel()

	template := `{{- $notification := index .Payload.Data "notification" -}}
{{- $recipient := index .Payload.Data "recipient" -}}
{
  "event": {{ .Input.Event | json }},
  "actor_id": {{ .Payload.ActorID | json }},
  "object_type": {{ .Payload.ObjectType | json }},
  "object_key": {{ .Payload.ObjectKey | json }},
  "detail": {{ .Payload.Detail | json }},
  "occurred_at": {{ .Payload.OccurredAt | json }},
  "url": {{ .Payload.URL | json }},
  "receiver": {{ .Receiver | json }},
  "title": {{ .Title | json }},
  "actor": {
    "id": {{ .Payload.Actor.ID | json }},
    "mention": {{ .Payload.Actor.Mention | json }},
    "display_name": {{ .Payload.Actor.DisplayName | json }},
    "email": {{ .Payload.Actor.Email | json }},
    "enabled": {{ .Payload.Actor.Enabled | json }}
  },
  "recipient": {
    "id": {{ .Payload.Recipient.ID | json }},
    "mention": {{ .Payload.Recipient.Mention | json }},
    "display_name": {{ .Payload.Recipient.DisplayName | json }},
    "email": {{ .Payload.Recipient.Email | json }},
    "enabled": {{ .Payload.Recipient.Enabled | json }}
  },
  "event_recipient": {
    "user_id": {{ index $recipient "user_id" | json }},
    "mention": {{ index $recipient "mention" | json }},
    "display_name": {{ index $recipient "display_name" | json }}
  },
  "notification": {
    "title": {{ index $notification "title" | json }},
    "body": {{ index $notification "body" | json }},
    "url": {{ index $notification "url" | json }},
    "source_type": {{ index $notification "source_type" | json }},
    "source_id": {{ index $notification "source_id" | json }},
    "source_name": {{ index $notification "source_name" | json }}
  }
}`

	require.NoError(t, validateWebhookInput(WebhookInput{
		Name:               "mail",
		URL:                "https://example.test/mail",
		BodyTemplate:       template,
		IncludeUserDetails: true,
	}, []string{EventNotificationCreated}))
}
