// Package metrics exposes Kumbuka's built-in Prometheus instrumentation.
package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry owns the private Prometheus registry and application metrics.
type Registry struct {
	// enabled reports whether Prometheus collection is active for this process.
	enabled bool
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

// NewRegistry constructs Kumbuka's private Prometheus registry when enabled.
func NewRegistry(enabled bool, version, commit string) *Registry {
	if !enabled {
		return &Registry{}
	}

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
		enabled:                  true,
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
	if !r.enabled {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

// InstrumentHTTP records request count and duration from the route selected by ServeMux.
func (r *Registry) InstrumentHTTP(handler http.Handler) http.Handler {
	if !r.enabled {
		return handler
	}

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		started := time.Now()
		tracked := &statusWriter{ResponseWriter: response}

		handler.ServeHTTP(tracked, request)

		method, route := requestLabels(request)
		status := tracked.status
		if status == 0 {
			status = http.StatusOK
		}

		r.httpRequests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
		r.httpDuration.WithLabelValues(method, route).Observe(time.Since(started).Seconds())
	})
}

// statusWriter records the final HTTP status while preserving response-controller access to the underlying writer.
type statusWriter struct {
	// ResponseWriter forwards response operations to the wrapped HTTP stack.
	http.ResponseWriter
	// status stores the first final response status.
	status int
}

// Unwrap exposes the underlying response writer to http.ResponseController.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteHeader records the first final response status while forwarding informational responses unchanged.
func (w *statusWriter) WriteHeader(status int) {
	if status >= 200 && w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write records the implicit successful status before forwarding response bytes.
func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

// ObservePluginInvocation records one executable plugin invocation.
func (r *Registry) ObservePluginInvocation(pluginID, moduleID, stage string, duration time.Duration, err error) {
	if !r.enabled {
		return
	}

	labels := []string{pluginID, moduleID, stage}
	r.pluginInvocations.WithLabelValues(labels...).Inc()
	r.pluginInvocationDuration.WithLabelValues(labels...).Observe(duration.Seconds())
	if err != nil {
		r.pluginInvocationErrors.WithLabelValues(labels...).Inc()
	}
}

// requestLabels returns bounded method and matched-route labels without exposing the raw request path.
func requestLabels(request *http.Request) (string, string) {
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	pattern := strings.TrimSpace(request.Pattern)
	if _, route, ok := strings.Cut(pattern, " "); ok {
		pattern = strings.TrimSpace(route)
	}
	if pattern == "" {
		pattern = "unmatched"
	}

	return method, pattern
}
