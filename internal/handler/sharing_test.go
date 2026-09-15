package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPublicShareHeaders(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()

	publicShareHeaders(response)

	assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	assert.Equal(t, "no-referrer", response.Header().Get("Referrer-Policy"))
	assert.Equal(t, "noindex, nofollow, noarchive", response.Header().Get("X-Robots-Tag"))
}
