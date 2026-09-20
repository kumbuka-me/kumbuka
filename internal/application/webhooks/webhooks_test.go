package webhooks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type webhookRepositoryStub struct {
	items       []domain.Webhook
	deliveries  []domain.WebhookDelivery
	saved       domain.Webhook
	deliveryErr error
}

func (r *webhookRepositoryStub) Webhooks(context.Context) ([]domain.Webhook, error) {
	return append([]domain.Webhook(nil), r.items...), nil
}

func (r *webhookRepositoryStub) Webhook(_ context.Context, id int64) (domain.Webhook, error) {
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return domain.Webhook{}, domain.ErrNotFound
}

func (r *webhookRepositoryStub) SaveWebhook(_ context.Context, id int64, item domain.Webhook) (domain.Webhook, error) {
	r.saved = item
	if id == 0 {
		item.ID = 1
	} else {
		item.ID = id
	}
	return item, nil
}

func (*webhookRepositoryStub) DeleteWebhook(context.Context, int64) error { return nil }

func (r *webhookRepositoryStub) AddWebhookDelivery(_ context.Context, id int64, event string, status, attempts int, message string) error {
	r.deliveries = append(r.deliveries, domain.WebhookDelivery{
		WebhookID:  id,
		Event:      event,
		StatusCode: status,
		Attempts:   attempts,
		Error:      message,
	})
	return r.deliveryErr
}

func (r *webhookRepositoryStub) WebhookDeliveries(context.Context, int) ([]domain.WebhookDelivery, error) {
	return append([]domain.WebhookDelivery(nil), r.deliveries...), nil
}

func testWebhookSecretCipher(t *testing.T) *secrets.Cipher {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte("01234567890123456789012345678901"))
	cipher, err := secrets.New(key)
	require.NoError(t, err)
	return cipher
}

func testWebhookLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWebhookDeliveryHistoryFailureIsObservableWithoutReplacingPrimaryError(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	repository := &webhookRepositoryStub{
		deliveryErr: errors.New("delivery history unavailable"),
		items: []domain.Webhook{{
			ID:           9,
			Name:         "protected",
			URL:          "https://example.invalid/hook",
			Events:       []string{"page.updated"},
			BodyTemplate: `{"event": {{ .Input.Event | json }}}`,
			Headers:      []domain.WebhookHeader{{Name: "Authorization", Value: "encrypted", Sensitive: true}},
			Enabled:      true,
		}},
	}

	err := NewWebhooks(repository, nil, logger, "").Emit(context.Background(), OutgoingEvent{Event: "page.updated"})

	require.ErrorIs(t, err, secrets.ErrNotConfigured)
	assert.Contains(t, output.String(), `"event":"webhook_delivery_record_failed"`)
	assert.Contains(t, output.String(), "delivery history unavailable")
}

func TestWebhooks(t *testing.T) {
	t.Parallel()

	t.Run("defaults new webhooks to explicit no-retry delivery", func(t *testing.T) {
		t.Parallel()

		item := DefaultWebhook()

		assert.False(t, item.RetryEnabled)
		assert.Equal(t, 2, item.RetryCount)
		assert.Equal(t, time.Second, item.RetryBackoff)
		assert.Equal(t, 30*time.Second, item.RetryMaxBackoff)
		assert.True(t, item.RetryJitter)
	})

	t.Run("renders custom payload and decrypts request headers", func(t *testing.T) {
		t.Parallel()

		type receivedWebhook struct {
			authorization string
			event         string
			body          []byte
		}
		received := make(chan receivedWebhook, 1)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			received <- receivedWebhook{
				authorization: r.Header.Get("Authorization"),
				event:         r.Header.Get("X-Kumbuka-Event"),
				body:          body,
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		cipher := testWebhookSecretCipher(t)
		encrypted, err := cipher.Encrypt("Bearer secret-token")
		require.NoError(t, err)
		repository := &webhookRepositoryStub{items: []domain.Webhook{{
			ID:           1,
			Name:         "slack",
			URL:          server.URL,
			Events:       []string{"page.updated"},
			BodyTemplate: `{"event": {{ .Input.Event | json }}, "page": {{ .Payload.ObjectKey | json }}, "url": {{ .Payload.URL | json }}}`,
			Headers: []domain.WebhookHeader{{
				ID:        7,
				Name:      "Authorization",
				Value:     encrypted,
				Sensitive: true,
			}},
			Enabled: true,
		}}}

		err = NewWebhooks(repository, cipher, testWebhookLogger(), "https://kumbuka.example").Emit(context.Background(), OutgoingEvent{
			Event:      "page.updated",
			ActorID:    42,
			ObjectType: "page",
			ObjectKey:  "guides/example",
			Detail:     "Example",
		})

		require.NoError(t, err)
		request := <-received
		var gotBody map[string]any
		require.NoError(t, json.Unmarshal(request.body, &gotBody))
		assert.Equal(t, "Bearer secret-token", request.authorization)
		assert.Equal(t, "page.updated", request.event)
		assert.Equal(t, "page.updated", gotBody["event"])
		assert.Equal(t, "guides/example", gotBody["page"])
		assert.Equal(t, "https://kumbuka.example/pages/guides/example", gotBody["url"])
		require.Len(t, repository.deliveries, 1)
		assert.Equal(t, http.StatusNoContent, repository.deliveries[0].StatusCode)
		assert.Equal(t, 1, repository.deliveries[0].Attempts)
	})

	t.Run("retries transient webhook failures with notifykit default policy", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if calls.Add(1) == 1 {
				http.Error(w, "retry", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()

		repository := &webhookRepositoryStub{items: []domain.Webhook{{
			ID:              1,
			Name:            "retry",
			URL:             server.URL,
			Events:          []string{"page.updated"},
			BodyTemplate:    `{"event": {{ .Input.Event | json }}}`,
			RetryEnabled:    true,
			RetryCount:      1,
			RetryBackoff:    time.Nanosecond,
			RetryMaxBackoff: time.Nanosecond,
			Enabled:         true,
		}}}

		err := NewWebhooks(repository, testWebhookSecretCipher(t), testWebhookLogger(), "").Emit(context.Background(), OutgoingEvent{
			Event:      "page.updated",
			ObjectType: "page",
			ObjectKey:  "guide",
		})

		require.NoError(t, err)
		assert.Equal(t, int32(2), calls.Load())
		require.Len(t, repository.deliveries, 1)
		assert.Equal(t, 2, repository.deliveries[0].Attempts)
		assert.Equal(t, http.StatusNoContent, repository.deliveries[0].StatusCode)
	})

	t.Run("does not retry when retries are disabled", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			http.Error(w, "retry", http.StatusInternalServerError)
		}))
		defer server.Close()

		repository := &webhookRepositoryStub{items: []domain.Webhook{{
			ID:           1,
			Name:         "single-attempt",
			URL:          server.URL,
			Events:       []string{"page.updated"},
			BodyTemplate: `{"event": {{ .Input.Event | json }}}`,
			RetryEnabled: false,
			RetryCount:   3,
			Enabled:      true,
		}}}

		err := NewWebhooks(repository, testWebhookSecretCipher(t), testWebhookLogger(), "").Emit(context.Background(), OutgoingEvent{
			Event:      "page.updated",
			ObjectType: "page",
			ObjectKey:  "guide",
		})

		require.Error(t, err)
		assert.Equal(t, int32(1), calls.Load())
		require.Len(t, repository.deliveries, 1)
		assert.Equal(t, 1, repository.deliveries[0].Attempts)
		assert.Equal(t, http.StatusInternalServerError, repository.deliveries[0].StatusCode)
	})

	t.Run("does not retry permanent client responses", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			http.Error(w, "bad request", http.StatusBadRequest)
		}))
		defer server.Close()

		repository := &webhookRepositoryStub{items: []domain.Webhook{{
			ID:              1,
			Name:            "client-error",
			URL:             server.URL,
			Events:          []string{"page.updated"},
			BodyTemplate:    `{"event": {{ .Input.Event | json }}}`,
			RetryEnabled:    true,
			RetryCount:      3,
			RetryBackoff:    time.Nanosecond,
			RetryMaxBackoff: time.Nanosecond,
			Enabled:         true,
		}}}

		err := NewWebhooks(repository, testWebhookSecretCipher(t), testWebhookLogger(), "").Emit(context.Background(), OutgoingEvent{
			Event:      "page.updated",
			ObjectType: "page",
			ObjectKey:  "guide",
		})

		require.Error(t, err)
		assert.Equal(t, int32(1), calls.Load())
		require.Len(t, repository.deliveries, 1)
		assert.Equal(t, 1, repository.deliveries[0].Attempts)
		assert.Equal(t, http.StatusBadRequest, repository.deliveries[0].StatusCode)
	})

	t.Run("redacts sensitive headers for administration", func(t *testing.T) {
		t.Parallel()

		repository := &webhookRepositoryStub{items: []domain.Webhook{{
			ID:   1,
			Name: "hook",
			Headers: []domain.WebhookHeader{{
				ID:        3,
				Name:      "Authorization",
				Value:     "encrypted-value",
				Sensitive: true,
			}},
		}}}

		items, err := NewWebhooks(repository, testWebhookSecretCipher(t), testWebhookLogger(), "").Webhooks(context.Background())

		require.NoError(t, err)
		require.Len(t, items, 1)
		require.Len(t, items[0].Headers, 1)
		assert.Empty(t, items[0].Headers[0].Value)
		assert.True(t, items[0].Headers[0].Configured)
		assert.NotEmpty(t, items[0].BodyTemplate)
	})

	t.Run("encrypts sensitive header values when saving", func(t *testing.T) {
		t.Parallel()

		cipher := testWebhookSecretCipher(t)
		repository := &webhookRepositoryStub{}
		_, err := NewWebhooks(repository, cipher, testWebhookLogger(), "").SaveWebhook(context.Background(), 0, WebhookInput{
			Name:            "hook",
			URL:             "https://example.test/hook",
			Events:          []string{"page.updated"},
			BodyTemplate:    `{"event": {{ .Input.Event | json }}}`,
			Headers:         []WebhookHeaderInput{{Name: "authorization", Value: "Bearer secret", Sensitive: true}},
			RetryEnabled:    true,
			RetryCount:      2,
			RetryBackoff:    time.Second,
			RetryMaxBackoff: 30 * time.Second,
			RetryJitter:     true,
			Enabled:         true,
		})

		require.NoError(t, err)
		require.Len(t, repository.saved.Headers, 1)
		assert.Equal(t, "Authorization", repository.saved.Headers[0].Name)
		assert.NotEqual(t, "Bearer secret", repository.saved.Headers[0].Value)
		plain, err := cipher.Decrypt(repository.saved.Headers[0].Value)
		require.NoError(t, err)
		assert.Equal(t, "Bearer secret", plain)
	})

	t.Run("preserves an unchanged stored sensitive header", func(t *testing.T) {
		t.Parallel()

		cipher := testWebhookSecretCipher(t)
		encrypted, err := cipher.Encrypt("Bearer stored")
		require.NoError(t, err)
		repository := &webhookRepositoryStub{items: []domain.Webhook{{
			ID:      9,
			Name:    "hook",
			Headers: []domain.WebhookHeader{{ID: 4, Name: "Authorization", Value: encrypted, Sensitive: true}},
		}}}

		_, err = NewWebhooks(repository, cipher, testWebhookLogger(), "").SaveWebhook(context.Background(), 9, WebhookInput{
			Name:            "hook",
			URL:             "https://example.test/hook",
			Events:          []string{"page.updated"},
			BodyTemplate:    `{"event": {{ .Input.Event | json }}}`,
			Headers:         []WebhookHeaderInput{{ID: 4, Name: "Authorization", Sensitive: true}},
			RetryCount:      2,
			RetryBackoff:    time.Second,
			RetryMaxBackoff: 30 * time.Second,
			Enabled:         true,
		})

		require.NoError(t, err)
		require.Len(t, repository.saved.Headers, 1)
		assert.Equal(t, encrypted, repository.saved.Headers[0].Value)
	})

	t.Run("rejects a payload template that cannot render", func(t *testing.T) {
		t.Parallel()

		repository := &webhookRepositoryStub{}
		_, err := NewWebhooks(repository, testWebhookSecretCipher(t), testWebhookLogger(), "").SaveWebhook(context.Background(), 0, WebhookInput{
			Name:            "hook",
			URL:             "https://example.test/hook",
			Events:          []string{"page.updated"},
			BodyTemplate:    `{"missing": {{ .Payload.Missing | json }}}`,
			RetryCount:      2,
			RetryBackoff:    time.Second,
			RetryMaxBackoff: 30 * time.Second,
			Enabled:         true,
		})

		validation, ok := err.(*domain.ValidationError)
		require.True(t, ok)
		require.NotEmpty(t, validation.Fields)
		assert.Equal(t, "body_template", validation.Fields[len(validation.Fields)-1].Field)
	})

	t.Run("reveals a sensitive header", func(t *testing.T) {
		t.Parallel()

		cipher := testWebhookSecretCipher(t)
		encrypted, err := cipher.Encrypt("Bearer stored")
		require.NoError(t, err)
		repository := &webhookRepositoryStub{items: []domain.Webhook{{
			ID:      9,
			Headers: []domain.WebhookHeader{{ID: 4, Name: "Authorization", Value: encrypted, Sensitive: true}},
		}}}

		value, err := NewWebhooks(repository, cipher, testWebhookLogger(), "").RevealWebhookHeader(context.Background(), 9, 4)

		require.NoError(t, err)
		assert.Equal(t, "Bearer stored", value)
	})
}
