package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMethodNotAllowed(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	methodNotAllowed(http.MethodGet, http.MethodHead).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/healthz", nil),
	)

	assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
	assert.Equal(t, "GET, HEAD", response.Header().Get("Allow"))
}
