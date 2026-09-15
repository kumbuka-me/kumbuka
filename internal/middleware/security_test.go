package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityHeaders(t *testing.T) {
	t.Parallel()

	handler := SecurityHeaders()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", response.Header().Get("X-Frame-Options"))
	assert.Equal(t, "same-origin", response.Header().Get("Referrer-Policy"))
	assert.Contains(t, response.Header().Get("Content-Security-Policy"), "frame-src 'self' blob:")
}

func TestRejectCrossSiteWrites(t *testing.T) {
	t.Parallel()

	t.Run("allows safe cross-site requests", func(t *testing.T) {
		t.Parallel()

		handler := RejectCrossSiteWrites(discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		request := httptest.NewRequest(http.MethodGet, "/pages/example", nil)

		request.Header.Set("Sec-Fetch-Site", "cross-site")

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("allows same-site writes", func(t *testing.T) {
		t.Parallel()

		handler := RejectCrossSiteWrites(discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		request := httptest.NewRequest(http.MethodPost, "/pages/example", nil)

		request.Header.Set("Sec-Fetch-Site", "same-origin")

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("rejects browser writes with a JSON problem", func(t *testing.T) {
		t.Parallel()

		handler := RejectCrossSiteWrites(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}))
		request := httptest.NewRequest(http.MethodPost, "/pages/example", nil)

		request.Header.Set("Sec-Fetch-Site", "cross-site")

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.JSONEq(t, `{"error":"Forbidden.","problems":{}}`, response.Body.String())
	})

	t.Run("rejects API writes with a JSON problem", func(t *testing.T) {
		t.Parallel()

		handler := RejectCrossSiteWrites(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}))
		request := httptest.NewRequest(http.MethodDelete, "/api/pages/example", nil)

		request.Header.Set("Sec-Fetch-Site", "cross-site")

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.JSONEq(
			t,
			`{"error":"Forbidden.","problems":{"request":"Cross-site write requests are not allowed."}}`,
			response.Body.String(),
		)
	})

	t.Run("logs rejected writes", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&output, nil))
		handler := RejectCrossSiteWrites(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}))
		request := httptest.NewRequest(http.MethodPatch, "/api/pages/example", nil)

		request.Header.Set("Sec-Fetch-Site", "cross-site")
		request.Header.Set("Origin", "https://attacker.example")

		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		logLine := output.String()

		assert.Contains(t, logLine, "event=cross_site_write_rejected")
		assert.Contains(t, logLine, "method=PATCH")
		assert.Contains(t, logLine, "path=/api/pages/example")
		assert.Contains(t, logLine, "origin=https://attacker.example")
		assert.Contains(t, logLine, "sec_fetch_site=cross-site")
	})
}

func TestIsSafeMethod(t *testing.T) {
	t.Parallel()

	assert.True(t, isSafeMethod(http.MethodGet))
	assert.True(t, isSafeMethod(http.MethodHead))
	assert.True(t, isSafeMethod(http.MethodOptions))
	assert.False(t, isSafeMethod(http.MethodPost))
	assert.False(t, isSafeMethod(http.MethodPut))
	assert.False(t, isSafeMethod(http.MethodPatch))
	assert.False(t, isSafeMethod(http.MethodDelete))
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), nil))
}
