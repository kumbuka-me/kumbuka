package endpoint

import "net/http"

// Metrics supplies the Prometheus exposition handler and HTTP route instrumentation.
type Metrics interface {
	// Metrics returns the Prometheus exposition handler.
	Metrics() http.Handler
	// InstrumentHandler wraps one stable HTTP route with request instrumentation.
	InstrumentHandler(string, http.Handler) http.Handler
}
