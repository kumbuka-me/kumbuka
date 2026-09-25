package endpoint

import "net/http"

// Metrics supplies Prometheus exposition and HTTP request instrumentation.
type Metrics interface {
	// Metrics returns the Prometheus exposition handler.
	Metrics() http.Handler
	// InstrumentHTTP wraps the completed HTTP application with request instrumentation.
	InstrumentHTTP(http.Handler) http.Handler
}
