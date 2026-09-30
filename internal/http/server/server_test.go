package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestRunStopsBothListenersOnBindFailure(t *testing.T) {
	for _, failApplication := range []bool{true, false} {
		occupied, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = occupied.Close() })
		applicationAddress, managementAddress := "127.0.0.1:0", occupied.Addr().String()
		if failApplication {
			applicationAddress, managementAddress = managementAddress, applicationAddress
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = Run(ctx, applicationAddress, managementAddress, Handlers{
			Application: http.NotFoundHandler(), Management: http.NotFoundHandler(),
		}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		require.Error(t, err)
		require.NoError(t, ctx.Err(), "listener failure should stop the peer before the deadline")
	}
}
