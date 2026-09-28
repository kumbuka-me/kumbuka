package middleware

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
)

const performanceTimingCookie = "kumbuka_perf"

// ServerTiming exposes opt-in request timings to browser developer tools.
// The frontend performance helper enables it with a same-site cookie, so normal
// requests pay only for one cookie lookup and avoid collecting render traces.
func ServerTiming() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if !serverTimingEnabled(request) {
				next.ServeHTTP(response, request)
				return
			}

			trace := renderprofile.New()
			request = request.WithContext(renderprofile.WithContext(request.Context(), trace))
			writer := &serverTimingWriter{
				ResponseWriter: response,
				trace:          trace,
				started:        time.Now(),
			}

			next.ServeHTTP(writer, request)
			writer.addHeader()
		})
	}
}

// serverTimingEnabled reports whether the browser opted into request profiling.
func serverTimingEnabled(request *http.Request) bool {
	cookie, err := request.Cookie(performanceTimingCookie)
	return err == nil && cookie.Value == "1"
}

// serverTimingWriter adds the final Server-Timing value before response headers are committed.
type serverTimingWriter struct {
	http.ResponseWriter
	trace     *renderprofile.Trace
	started   time.Time
	committed bool
}

// Unwrap exposes optional transport capabilities through http.ResponseController.
func (w *serverTimingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteHeader commits timing metadata with the final response status.
func (w *serverTimingWriter) WriteHeader(status int) {
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

	w.addHeader()
	w.committed = true
	w.ResponseWriter.WriteHeader(status)
}

// Write supplies an implicit successful status before writing response data.
func (w *serverTimingWriter) Write(body []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// addHeader snapshots the current render trace and appends browser-readable timing metrics.
func (w *serverTimingWriter) addHeader() {
	if w.committed {
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

// serverTimingMetric formats one Server-Timing metric in milliseconds.
func serverTimingMetric(name string, duration time.Duration) string {
	return fmt.Sprintf("%s;dur=%.3f", name, float64(duration)/float64(time.Millisecond))
}
