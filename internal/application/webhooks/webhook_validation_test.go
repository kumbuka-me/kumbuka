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
		BodyTemplate:    `{"value": {{ .Missing }}}`,
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
