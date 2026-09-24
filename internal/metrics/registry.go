// Package metrics exposes Kumbuka's built-in Prometheus instrumentation.
package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry owns the private Prometheus registry and application metrics.
type Registry struct {
	// registry contains only collectors explicitly registered by Kumbuka.
	registry *prometheus.Registry
	// httpRequests counts completed HTTP requests by stable route and response code.
	httpRequests *prometheus.CounterVec
	// httpDuration observes HTTP request duration by stable route.
	httpDuration *prometheus.HistogramVec
	// pluginInvocations counts executable plugin calls by plugin module and stage.
	pluginInvocations *prometheus.CounterVec
	// pluginInvocationErrors counts failed executable plugin calls.
	pluginInvocationErrors *prometheus.CounterVec
	// pluginInvocationDuration observes executable plugin call duration.
	pluginInvocationDuration *prometheus.HistogramVec
}

// NewRegistry constructs Kumbuka's private Prometheus registry.
func NewRegistry(version, commit string) *Registry {
	registry := prometheus.NewRegistry()

	httpRequests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kumbuka_http_requests_total",
			Help: "Total number of completed HTTP requests by method, stable route, and status code.",
		},
		[]string{"method", "route", "code"},
	)
	httpDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kumbuka_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds by method and stable route.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)
	pluginInvocations := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kumbuka_plugin_invocations_total",
			Help: "Total number of executable plugin invocations by plugin, module, and stage.",
		},
		[]string{"plugin", "module", "stage"},
	)
	pluginInvocationErrors := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kumbuka_plugin_invocation_errors_total",
			Help: "Total number of failed executable plugin invocations by plugin, module, and stage.",
		},
		[]string{"plugin", "module", "stage"},
	)
	pluginInvocationDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kumbuka_plugin_invocation_duration_seconds",
			Help:    "Executable plugin invocation duration in seconds by plugin, module, and stage.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"plugin", "module", "stage"},
	)
	buildInfo := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "kumbuka_build_info",
		Help: "Kumbuka build information.",
		ConstLabels: prometheus.Labels{
			"version": version,
			"commit":  commit,
		},
	})
	buildInfo.Set(1)

	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		httpRequests,
		httpDuration,
		pluginInvocations,
		pluginInvocationErrors,
		pluginInvocationDuration,
		buildInfo,
	)

	return &Registry{
		registry:                 registry,
		httpRequests:             httpRequests,
		httpDuration:             httpDuration,
		pluginInvocations:        pluginInvocations,
		pluginInvocationErrors:   pluginInvocationErrors,
		pluginInvocationDuration: pluginInvocationDuration,
	}
}

// Metrics returns the Prometheus exposition handler for this registry.
func (r *Registry) Metrics() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

// InstrumentHandler records request count and duration using the registered route pattern rather than the raw URL.
func (r *Registry) InstrumentHandler(pattern string, handler http.Handler) http.Handler {
	method, route := routeLabels(pattern)
	labels := prometheus.Labels{"method": method, "route": route}

	counter := r.httpRequests.MustCurryWith(labels)
	duration := r.httpDuration.MustCurryWith(labels)

	return promhttp.InstrumentHandlerDuration(
		duration,
		promhttp.InstrumentHandlerCounter(counter, handler),
	)
}

// ObservePluginInvocation records one executable plugin invocation.
func (r *Registry) ObservePluginInvocation(pluginID, moduleID, stage string, duration time.Duration, err error) {
	labels := []string{pluginID, moduleID, stage}
	r.pluginInvocations.WithLabelValues(labels...).Inc()
	r.pluginInvocationDuration.WithLabelValues(labels...).Observe(duration.Seconds())
	if err != nil {
		r.pluginInvocationErrors.WithLabelValues(labels...).Inc()
	}
}

// routeLabels splits a ServeMux method/path pattern into bounded metric label values.
func routeLabels(pattern string) (string, string) {
	pattern = strings.TrimSpace(pattern)
	method, route, ok := strings.Cut(pattern, " ")
	if !ok || strings.TrimSpace(method) == "" || strings.TrimSpace(route) == "" {
		return "ANY", pattern
	}

	return strings.ToUpper(strings.TrimSpace(method)), strings.TrimSpace(route)
}
