package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformanceDiagnosticsTraceRequestWithoutBrowserTiming(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := PerformanceDiagnostics(logger)(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		trace := renderprofile.FromContext(request.Context())
		require.NotNil(t, trace)
		stop := trace.Measure("page_lookup")
		stop()
		_, _ = response.Write([]byte("ok"))
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pages/example", nil))

	assert.Empty(t, response.Header().Values("Server-Timing"))
	assert.Contains(t, logs.String(), "event=performance_timing")
	assert.Contains(t, logs.String(), "stages.page_lookup_ms=")
}

func TestPerformanceDiagnosticsExposeServerTimingForEnabledBrowser(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := PerformanceDiagnostics(logger)(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		trace := renderprofile.FromContext(request.Context())
		require.NotNil(t, trace)
		stop := trace.Measure("page_lookup")
		stop()
		_, _ = response.Write([]byte("ok"))
	}))

	request := httptest.NewRequest(http.MethodGet, "/pages/example", nil)
	request.AddCookie(&http.Cookie{Name: performanceTimingCookie, Value: "1"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	timing := response.Header().Get("Server-Timing")
	assert.Contains(t, timing, "kumbuka;dur=")
	assert.Contains(t, timing, "page_lookup;dur=")
	assert.Contains(t, logs.String(), "event=performance_timing")
}

func TestPerformanceDiagnosticsPreservesExistingServerTimingHeader(t *testing.T) {
	handler := PerformanceDiagnostics(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Add("Server-Timing", "proxy;dur=1.000")
		response.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: performanceTimingCookie, Value: "1"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	values := response.Header().Values("Server-Timing")
	require.Len(t, values, 2)
	assert.Equal(t, "proxy;dur=1.000", values[0])
	assert.Contains(t, values[1], "kumbuka;dur=")
}
