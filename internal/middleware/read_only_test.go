package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadOnly(t *testing.T) {
	t.Parallel()

	handler := ReadOnly()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("allows reads", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pages/example", nil))

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("blocks application writes", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/edit/example", nil))

		assert.Equal(t, http.StatusServiceUnavailable, response.Code)
		assert.Contains(t, response.Body.String(), "Kumbuka is running in read-only mode.")
	})

	t.Run("allows local login", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/local", nil))

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("allows logout", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/auth/logout", nil))

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("does not exempt similar application paths", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/authentication/settings", nil))

		assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	})
}
