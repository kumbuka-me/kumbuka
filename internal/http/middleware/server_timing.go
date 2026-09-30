package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
)

const performanceTimingCookie = "kumbuka_perf"

// PerformanceDiagnostics attaches one shared trace to each application request and logs it.
// The middleware is installed only when deployment-level diagnostics are enabled; browsers opt in separately to receive Server-Timing.
func PerformanceDiagnostics(logger *slog.Logger) Middleware {
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			exposeServerTiming := performanceDiagnosticsRequested(request)
			trace := renderprofile.New()
			request = request.WithContext(renderprofile.WithContext(request.Context(), trace))
			writer := &performanceWriter{
				ResponseWriter:     response,
				trace:              trace,
				started:            time.Now(),
				exposeServerTiming: exposeServerTiming,
			}
			defer writer.log(logger, request)

			next.ServeHTTP(writer, request)
			writer.addHeader()
		})
	}
}

// performanceDiagnosticsRequested reports whether this browser selected request profiling.
func performanceDiagnosticsRequested(request *http.Request) bool {
	cookie, err := request.Cookie(performanceTimingCookie)
	return err == nil && cookie.Value == "1"
}

// performanceWriter adds Server-Timing and records response metadata for the shared request profile.
type performanceWriter struct {
	// ResponseWriter receives the underlying HTTP response.
	http.ResponseWriter
	// trace collects request stages and plugin invocation timings.
	trace *renderprofile.Trace
	// started records when response profiling began.
	started time.Time
	// status is the final HTTP status, or zero before commitment.
	status int
	// bytes counts body bytes accepted by the underlying writer.
	bytes int
	// committed prevents changes after the final headers have been sent.
	committed bool
	// exposeServerTiming enables timing headers for opted-in browsers.
	exposeServerTiming bool
}

// Unwrap exposes optional transport capabilities through http.ResponseController.
func (w *performanceWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteHeader commits timing metadata with the final response status.
func (w *performanceWriter) WriteHeader(status int) {
	// Informational responses do not commit the final response headers. Keep the
	// timing header for the final status so Server-Timing describes the work that
	// led to the actual response.
	if status >= 100 && status < 200 && status != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.committed {
		return
	}

	w.status = status
	w.addHeader()
	w.committed = true
	w.ResponseWriter.WriteHeader(status)
}

// Write supplies an implicit successful status before writing response data.
func (w *performanceWriter) Write(body []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(body)
	w.bytes += written
	return written, err
}

// addHeader snapshots the current request trace and appends browser-readable timing metrics.
func (w *performanceWriter) addHeader() {
	if w.committed || !w.exposeServerTiming {
		return
	}

	metrics := []string{serverTimingMetric("kumbuka", time.Since(w.started))}
	snapshot := w.trace.Snapshot()
	stageNames := make([]string, 0, len(snapshot.Stages))
	for name := range snapshot.Stages {
		stageNames = append(stageNames, name)
	}
	sort.Strings(stageNames)
	for _, name := range stageNames {
		metrics = append(metrics, serverTimingMetric(name, snapshot.Stages[name]))
	}
	if snapshot.WASMSummary.Calls > 0 {
		metrics = append(metrics, serverTimingMetric("plugin_wasm", snapshot.WASMSummary.Total))
	}

	w.Header().Add("Server-Timing", strings.Join(metrics, ", "))
}

// log writes the same request trace exposed through Server-Timing as structured backend diagnostics.
func (w *performanceWriter) log(logger *slog.Logger, request *http.Request) {
	snapshot := w.trace.Snapshot()
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}

	for index, call := range snapshot.WASMCalls {
		logger.InfoContext(
			request.Context(),
			"performance WASM timing",
			"event", "performance_wasm_timing",
			"profile_id", snapshot.ID,
			"call", index+1,
			"plugin_id", call.PluginID,
			"module_id", call.ModuleID,
			"stage", call.Stage,
			"failed", call.Failed,
			"duration_ms", durationMilliseconds(call.Total),
			"gate_wait_ms", durationMilliseconds(call.GateWait),
			"instantiate_ms", durationMilliseconds(call.Instantiate),
			"encode_ms", durationMilliseconds(call.Encode),
			"allocate_ms", durationMilliseconds(call.Allocate),
			"memory_write_ms", durationMilliseconds(call.MemoryWrite),
			"guest_execute_ms", durationMilliseconds(call.Execute),
			"memory_read_ms", durationMilliseconds(call.MemoryRead),
			"decode_ms", durationMilliseconds(call.Decode),
			"validate_ms", durationMilliseconds(call.Validate),
			"request_bytes", call.RequestBytes,
			"response_bytes", call.ResponseBytes,
		)
	}

	stageNames := make([]string, 0, len(snapshot.Stages))
	for name := range snapshot.Stages {
		stageNames = append(stageNames, name)
	}
	sort.Strings(stageNames)
	stageAttributes := make([]any, 0, len(stageNames)*2)
	for _, name := range stageNames {
		stageAttributes = append(stageAttributes, name+"_ms", durationMilliseconds(snapshot.Stages[name]))
	}

	summary := snapshot.WASMSummary
	args := []any{
		"event", "performance_timing",
		"profile_id", snapshot.ID,
		"method", request.Method,
		"path", request.URL.Path,
		"status", status,
		"duration_ms", durationMilliseconds(snapshot.Duration),
		"response_bytes", w.bytes,
		"wasm_calls", summary.Calls,
		"wasm_calls_logged", len(snapshot.WASMCalls),
		"wasm_calls_dropped", snapshot.DroppedWASMCalls,
		"wasm_request_bytes", summary.RequestBytes,
		"wasm_response_bytes", summary.ResponseBytes,
		"wasm_total_ms", durationMilliseconds(summary.Total),
		"wasm_gate_wait_ms", durationMilliseconds(summary.GateWait),
		"wasm_instantiate_ms", durationMilliseconds(summary.Instantiate),
		"wasm_encode_ms", durationMilliseconds(summary.Encode),
		"wasm_allocate_ms", durationMilliseconds(summary.Allocate),
		"wasm_memory_write_ms", durationMilliseconds(summary.MemoryWrite),
		"wasm_guest_execute_ms", durationMilliseconds(summary.Execute),
		"wasm_memory_read_ms", durationMilliseconds(summary.MemoryRead),
		"wasm_decode_ms", durationMilliseconds(summary.Decode),
		"wasm_validate_ms", durationMilliseconds(summary.Validate),
	}
	if request.Pattern != "" {
		args = append(args, "route", request.Pattern)
	}
	if len(stageAttributes) != 0 {
		args = append(args, slog.Group("stages", stageAttributes...))
	}

	logger.InfoContext(request.Context(), "performance timing", args...)
}

// serverTimingMetric formats one Server-Timing metric in milliseconds.
func serverTimingMetric(name string, duration time.Duration) string {
	return fmt.Sprintf("%s;dur=%.3f", name, durationMilliseconds(duration))
}

// durationMilliseconds converts a duration to a floating-point millisecond value.
func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
