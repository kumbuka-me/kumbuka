package routes

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGlobalSecurityHeadersCoverMiddlewareRejections(t *testing.T) {
	t.Parallel()

	t.Run("cross-site write", func(t *testing.T) {
		t.Parallel()

		router := New(Config{Logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})
		router.HandleFunc("POST /write", func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "rejected request must not reach the endpoint")
		})
		request := httptest.NewRequest(http.MethodPost, "/write", nil)
		request.Header.Set("Sec-Fetch-Site", "cross-site")
		response := httptest.NewRecorder()

		router.Handler().ServeHTTP(response, request)

		assert.Equal(t, http.StatusForbidden, response.Code)
		assertSecurityHeaders(t, response.Header())
	})

	t.Run("read-only write", func(t *testing.T) {
		t.Parallel()

		router := New(Config{
			Logger:   slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
			ReadOnly: true,
		})
		router.HandleFunc("POST /write", func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "rejected request must not reach the endpoint")
		})
		response := httptest.NewRecorder()

		router.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/write", nil))

		assert.Equal(t, http.StatusServiceUnavailable, response.Code)
		assertSecurityHeaders(t, response.Header())
	})
}

func assertSecurityHeaders(t *testing.T, header http.Header) {
	t.Helper()

	assert.Equal(t, "nosniff", header.Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", header.Get("X-Frame-Options"))
	assert.Equal(t, "same-origin", header.Get("Referrer-Policy"))
	assert.NotEmpty(t, header.Get("Content-Security-Policy"))
}

// metricsStub records route instrumentation performed by Router.
type metricsStub struct {
	// pattern records the route pattern supplied to the test double.
	pattern string
}

// Metrics returns a placeholder exposition handler for the test double.
func (*metricsStub) Metrics() http.Handler { return http.NotFoundHandler() }

// InstrumentHandler records the pattern and marks requests that reached the wrapped route.
func (m *metricsStub) InstrumentHandler(pattern string, handler http.Handler) http.Handler {
	m.pattern = pattern
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Metrics-Instrumented", "true")
		handler.ServeHTTP(response, request)
	})
}

func TestRegisteredRoutesUseMetricsInstrumentation(t *testing.T) {
	t.Parallel()

	metrics := &metricsStub{}
	router := New(Config{
		Logger:  slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		Metrics: metrics,
	})
	router.HandleFunc("GET /example/{id}", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
	response := httptest.NewRecorder()

	router.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/example/42", nil))

	assert.Equal(t, "GET /example/{id}", metrics.pattern)
	assert.Equal(t, "true", response.Header().Get("X-Metrics-Instrumented"))
	assert.Equal(t, http.StatusNoContent, response.Code)
}
