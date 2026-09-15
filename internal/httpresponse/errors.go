package httpresponse

import (
	"log/slog"
	"net/http"

	"github.com/containeroo/uuidv7"
)

// RequestWriter retains diagnostic context for the request and its access log.
// Unwrap lets http.ResponseController reach optional transport capabilities.
type RequestWriter struct {
	http.ResponseWriter
	method         string
	path           string
	ErrorReference string
	Status         int
}

// NewRequestWriter wraps a response writer with request and status diagnostics.
func NewRequestWriter(w http.ResponseWriter, r *http.Request) *RequestWriter {
	return &RequestWriter{ResponseWriter: w, method: r.Method, path: r.URL.Path}
}

// Unwrap returns the underlying response writer for response controllers.
func (w *RequestWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// WriteHeader records the final status without treating informational headers as committed.
func (w *RequestWriter) WriteHeader(status int) {
	if w.Status != 0 {
		return
	}
	if status >= 200 {
		w.Status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write records an implicit successful status before writing response data.
func (w *RequestWriter) Write(data []byte) (int, error) {
	if w.Status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

// FlushError flushes the underlying response while preserving recorded status.
func (w *RequestWriter) FlushError() error {
	if w.Status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

// InternalServerError logs a failure and writes a safe HTTP 500 response with a reference ID.
func InternalServerError(logger *slog.Logger, w http.ResponseWriter, err error) {
	if request, ok := w.(*RequestWriter); ok {
		logger = logger.With("method", request.method, "path", request.path)
	}
	reference, referenceErr := uuidv7.New()
	if referenceErr != nil {
		logger.Error(
			"generate error reference",
			"event", "error_reference_failed",
			"error", referenceErr,
		)
		logger.Error(
			"request failed",
			"event", "request_failed",
			"error", err,
		)

		if request, ok := w.(*RequestWriter); ok && request.Status != 0 {
			return
		}
		Problem(w,
			http.StatusInternalServerError,
			"The request could not be processed.",
		)
		return
	}

	if request, ok := w.(*RequestWriter); ok {
		request.ErrorReference = reference
	}
	logger.Error(
		"request failed",
		"event", "request_failed",
		"error_reference", reference,
		"error", err,
	)

	if request, ok := w.(*RequestWriter); ok && request.Status != 0 {
		return
	}
	Problem(w,
		http.StatusInternalServerError,
		"The request could not be processed. Reference: "+reference,
	)
}
