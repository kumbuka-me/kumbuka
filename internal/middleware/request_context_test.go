package middleware

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestFailureCorrelation(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panics], func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			response := httptest.NewRecorder()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if panics {
					panic("private diagnostic")
				}
				httpresponse.InternalServerError(logger, w, errors.New("private diagnostic"))
			})
			Chain(handler, RequestContext(), AccessLog(logger), RecoverPanics(logger)).ServeHTTP(response, httptest.NewRequest("GET", "/pages/example?secret=hidden", nil))
			require.Equal(t, 500, response.Code)
			assert.NotContains(t, response.Body.String(), "private diagnostic")
			assert.Contains(t, logs.String(), `"method":"GET"`)
			assert.Contains(t, logs.String(), `"path":"/pages/example"`)
			assert.NotContains(t, logs.String(), "hidden")
			assert.Equal(t, 2, bytes.Count(logs.Bytes(), []byte(`"error_reference":`)))
			assert.Contains(t, logs.String(), "private diagnostic")
		})
	}
}

func TestPanicAfterResponseAbortsWithoutAppendingProblem(t *testing.T) {
	response := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("started"))
		panic("failed after write")
	})
	assert.PanicsWithValue(t, http.ErrAbortHandler, func() {
		Chain(handler, RequestContext(), RecoverPanics(slog.Default())).ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	})
	assert.Equal(t, "started", response.Body.String())
}
