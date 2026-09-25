package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	appsystem "github.com/kumbuka-me/kumbuka/internal/application/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// operationalSystemRepositoryStub supplies successful health state for operational-route tests.
type operationalSystemRepositoryStub struct{}

func (operationalSystemRepositoryStub) DatabaseSize(context.Context) (int64, error) { return 0, nil }
func (operationalSystemRepositoryStub) Ping(context.Context) error                  { return nil }
func (operationalSystemRepositoryStub) SetupRequired(context.Context) (bool, error) {
	return false, nil
}
func (operationalSystemRepositoryStub) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

// operationalMetricsStub marks requests that pass through HTTP instrumentation.
type operationalMetricsStub struct{}

func (operationalMetricsStub) Metrics() http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	})
}

func (operationalMetricsStub) InstrumentHTTP(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Metrics-Instrumented", "true")
		handler.ServeHTTP(response, request)
	})
}

func TestOperationalRoutesBypassHTTPMetrics(t *testing.T) {
	t.Parallel()

	application := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		assert.Empty(t, request.Pattern)
		response.WriteHeader(http.StatusAccepted)
	})
	config := Config{
		InfrastructureConfig: InfrastructureConfig{
			MetricsEnabled: true,
			Metrics:        operationalMetricsStub{},
		},
		AdministrationConfig: AdministrationConfig{
			System: appsystem.NewSystem(operationalSystemRepositoryStub{}),
		},
	}
	handler := operationalRoutes(application, config)

	for _, test := range []struct {
		name         string
		method       string
		path         string
		status       int
		instrumented bool
	}{
		{name: "metrics", method: http.MethodGet, path: "/metrics", status: http.StatusNoContent},
		{name: "health", method: http.MethodGet, path: "/healthz", status: http.StatusOK},
		{name: "metrics method rejected", method: http.MethodPost, path: "/metrics", status: http.StatusMethodNotAllowed},
		{name: "health method rejected", method: http.MethodPost, path: "/healthz", status: http.StatusMethodNotAllowed},
		{name: "application", method: http.MethodGet, path: "/example", status: http.StatusAccepted, instrumented: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))

			require.Equal(t, test.status, response.Code)
			assert.Equal(t, test.instrumented, response.Header().Get("X-Metrics-Instrumented") == "true")
		})
	}
}

func TestOperationalRoutesHideMetricsWhenDisabled(t *testing.T) {
	t.Parallel()

	application := http.NotFoundHandler()
	config := Config{
		InfrastructureConfig: InfrastructureConfig{
			MetricsEnabled: false,
			Metrics:        operationalMetricsStub{},
		},
		AdministrationConfig: AdministrationConfig{
			System: appsystem.NewSystem(operationalSystemRepositoryStub{}),
		},
	}
	handler := operationalRoutes(application, config)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Empty(t, response.Header().Get("X-Metrics-Instrumented"))
}
