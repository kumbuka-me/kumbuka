package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeHealthStore struct {
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

	t.Run("Returns ok", func(t *testing.T) {
		t.Parallel()

		store := &fakeHealthStore{pingFn: func(context.Context) error { return nil }}

		handler := Health(store)
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
	})
}
