package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBearerAuthenticateHeaderValidation verifies that explicit malformed bearer credentials are rejected.
func TestBearerAuthenticateHeaderValidation(t *testing.T) {
	t.Parallel()

	t.Run("missing authorization header remains unauthenticated", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("GET", "/api/search", nil)
		_, err := NewBearer(nil).Authenticate(request)

		assert.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("empty bearer token is invalid credentials", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("GET", "/api/search", nil)

		request.Header.Set("Authorization", "Bearer")

		_, err := NewBearer(nil).Authenticate(request)

		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("whitespace-only bearer token is invalid credentials", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("GET", "/api/search", nil)
		request.Header["Authorization"] = []string{"Bearer   "}
		_, err := NewBearer(nil).Authenticate(request)

		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("wrong authorization scheme is invalid credentials", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("GET", "/api/search", nil)

		request.Header.Set("Authorization", "Basic abc")

		_, err := NewBearer(nil).Authenticate(request)

		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})
}
