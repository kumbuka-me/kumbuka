package endpoint

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appwebhooks "github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// webhookAdminSaveStub provides controllable webhook admin save behavior for tests.
type webhookAdminSaveStub struct {
	webhookAdminService
	// id records the ID observed by the test double.
	id int64
	// input records the input observed by the test double.
	input appwebhooks.WebhookInput
}

func (s *webhookAdminSaveStub) SaveWebhook(_ context.Context, id int64, input appwebhooks.WebhookInput) (domain.Webhook, error) {
	s.id = id
	s.input = input
	return domain.Webhook{ID: id}, nil
}

func TestSaveAdminWebhookParsesDeliveryConfiguration(t *testing.T) {
	t.Parallel()

	form := url.Values{
		"name":                        {"Slack"},
		"url":                         {"https://hooks.example.test/kumbuka"},
		"event":                       {"page.created", "page.updated"},
		"body_template":               {`{"text": {{ .Payload.Detail | json }}}`},
		"include_user_details":        {"on"},
		"enabled":                     {"on"},
		"retry_enabled":               {"on"},
		"retry_count":                 {"4"},
		"retry_backoff":               {"750ms"},
		"retry_max_backoff":           {"45s"},
		"retry_jitter":                {"on"},
		"webhook_header_row":          {"h7", "n1"},
		"webhook_header_h7_id":        {"7"},
		"webhook_header_h7_name":      {"Authorization"},
		"webhook_header_h7_value":     {""},
		"webhook_header_h7_sensitive": {"on"},
		"webhook_header_n1_name":      {"X-Environment"},
		"webhook_header_n1_value":     {"production"},
		"webhook_header_n1_sensitive": {""},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/webhooks/9", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("id", "9")
	response := httptest.NewRecorder()
	stub := &webhookAdminSaveStub{}

	SaveAdminWebhook(stub, slog.New(slog.NewTextHandler(io.Discard, nil)))(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/admin/webhooks", response.Header().Get("Location"))
	assert.Equal(t, int64(9), stub.id)
	assert.Equal(t, "Slack", stub.input.Name)
	assert.Equal(t, "https://hooks.example.test/kumbuka", stub.input.URL)
	assert.Equal(t, []string{"page.created", "page.updated"}, stub.input.Events)
	assert.Equal(t, `{"text": {{ .Payload.Detail | json }}}`, stub.input.BodyTemplate)
	assert.True(t, stub.input.IncludeUserDetails)
	assert.True(t, stub.input.Enabled)
	assert.True(t, stub.input.RetryEnabled)
	assert.Equal(t, 4, stub.input.RetryCount)
	assert.Equal(t, 750*time.Millisecond, stub.input.RetryBackoff)
	assert.Equal(t, 45*time.Second, stub.input.RetryMaxBackoff)
	assert.True(t, stub.input.RetryJitter)
	require.Len(t, stub.input.Headers, 2)
	assert.Equal(t, appwebhooks.WebhookHeaderInput{ID: 7, Name: "Authorization", Sensitive: true}, stub.input.Headers[0])
	assert.Equal(t, appwebhooks.WebhookHeaderInput{Name: "X-Environment", Value: "production"}, stub.input.Headers[1])
}

func TestWebhookHeadersFromFormRejectsDuplicateRows(t *testing.T) {
	t.Parallel()

	form := url.Values{
		"webhook_header_row": {"n1", "n1"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/webhooks", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, request.ParseForm())

	headers, err := webhookHeadersFromForm(request)

	require.Error(t, err)
	assert.Nil(t, headers)
}

func TestWebhookRetryFromFormRejectsInvalidDuration(t *testing.T) {
	t.Parallel()

	form := url.Values{
		"retry_count":       {"2"},
		"retry_backoff":     {"soon"},
		"retry_max_backoff": {"30s"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/webhooks", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, request.ParseForm())

	_, _, _, err := webhookRetryFromForm(request)

	require.Error(t, err)
}
