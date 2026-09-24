package metrics

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginProviderStub supplies deterministic plugin lifecycle state for metric tests.
type pluginProviderStub struct {
	// plugins contains the plugin metadata returned by the test double.
	plugins []plugin.LoadedPlugin
}

// Plugins returns the configured plugin metadata snapshot.
func (s pluginProviderStub) Plugins() []plugin.LoadedPlugin { return s.plugins }

func TestRegistryExposesBuildAndRuntimeMetrics(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("1.2.3", "abc123")
	output := scrapeMetrics(t, registry)

	assert.Contains(t, output, `kumbuka_build_info{commit="abc123",version="1.2.3"} 1`)
	assert.Contains(t, output, "go_goroutines")
	assert.Contains(t, output, "process_cpu_seconds_total")
}

func TestRegistryInstrumentsStableHTTPRoute(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("test", "test")
	handler := registry.InstrumentHandler(
		"GET /pages/{slug...}",
		http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusNoContent)
		}),
	)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pages/private/runbook", nil))
	require.Equal(t, http.StatusNoContent, response.Code)

	output := scrapeMetrics(t, registry)
	assert.Contains(t, output, `kumbuka_http_requests_total{code="204",method="GET",route="/pages/{slug...}"} 1`)
	assert.Contains(t, output, `kumbuka_http_request_duration_seconds_count{method="GET",route="/pages/{slug...}"} 1`)
	assert.NotContains(t, output, "private/runbook")
}

func TestRegistryObservesPluginInvocations(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("test", "test")
	registry.ObservePluginInvocation("me.kumbuka.callouts", "callouts", "preprocess", 25*time.Millisecond, nil)
	registry.ObservePluginInvocation("me.kumbuka.callouts", "callouts", "preprocess", 10*time.Millisecond, errors.New("failed"))

	output := scrapeMetrics(t, registry)
	assert.Contains(t, output, `kumbuka_plugin_invocations_total{module="callouts",plugin="me.kumbuka.callouts",stage="preprocess"} 2`)
	assert.Contains(t, output, `kumbuka_plugin_invocation_errors_total{module="callouts",plugin="me.kumbuka.callouts",stage="preprocess"} 1`)
	assert.Contains(t, output, `kumbuka_plugin_invocation_duration_seconds_count{module="callouts",plugin="me.kumbuka.callouts",stage="preprocess"} 2`)
}

func TestRegistryCollectsCurrentPluginCounts(t *testing.T) {
	t.Parallel()

	registry := NewRegistry("test", "test")
	registry.RegisterPluginProvider(pluginProviderStub{plugins: []plugin.LoadedPlugin{
		{Enabled: true, Manifest: pluginpackage.Manifest{ID: "me.kumbuka.enabled"}},
		{Enabled: false, Manifest: pluginpackage.Manifest{ID: "me.kumbuka.disabled"}},
	}})

	output := scrapeMetrics(t, registry)
	assert.Contains(t, output, "kumbuka_plugins_installed 2")
	assert.Contains(t, output, "kumbuka_plugins_enabled 1")
}

// scrapeMetrics returns the text exposition produced by the registry handler.
func scrapeMetrics(t *testing.T, registry *Registry) string {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	registry.Metrics().ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	body, err := io.ReadAll(response.Result().Body)
	require.NoError(t, err)
	require.NoError(t, response.Result().Body.Close())
	return strings.TrimSpace(string(body))
}
