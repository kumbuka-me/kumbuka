package endpoint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHealthStore provides test state for fake health store behavior.
type fakeHealthStore struct {
	// pingFn provides the callback invoked by the test double.
	pingFn func(context.Context) error
}

func (f *fakeHealthStore) Ping(ctx context.Context) error {
	if f.pingFn == nil {
		return nil
	}
	return f.pingFn(ctx)
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	Health().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"status":"ok"}`, response.Body.String())
}

func TestReadyz(t *testing.T) {
	t.Parallel()

	t.Run("returns ok when dependencies are ready", func(t *testing.T) {
		t.Parallel()

		store := &fakeHealthStore{pingFn: func(context.Context) error { return nil }}
		response := httptest.NewRecorder()

		Ready(store).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		require.Equal(t, http.StatusOK, response.Code)
		assert.JSONEq(t, `{"status":"ok"}`, response.Body.String())
	})

	t.Run("returns unavailable when a dependency is down", func(t *testing.T) {
		t.Parallel()

		store := &fakeHealthStore{pingFn: func(context.Context) error { return errors.New("database down") }}
		response := httptest.NewRecorder()

		Ready(store).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

		require.Equal(t, http.StatusServiceUnavailable, response.Code)
	})
}
