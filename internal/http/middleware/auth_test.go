package middleware

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authenticatorStub is a deterministic authenticator used to verify middleware fallback behavior.
type authenticatorStub struct {
	// user is returned when err is nil.
	user domain.User
	// err is the authentication result returned to the middleware.
	err error
	// calls counts Authenticate invocations.
	calls *int
}

// Authenticate returns the configured stub result.
func (a authenticatorStub) Authenticate(*http.Request) (domain.User, error) {
	if a.calls != nil {
		*a.calls++
	}
	return a.user, a.err
}

// TestAuthenticateAPIStopsOnInvalidCredentials verifies that explicit bad credentials never fall through to browser
// auth.
func TestAuthenticateAPIStopsOnInvalidCredentials(t *testing.T) {
	t.Parallel()

	firstCalls := 0
	fallbackCalls := 0
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := AuthenticateAPI(
		logger,
		authenticatorStub{err: auth.ErrInvalidCredentials, calls: &firstCalls},
		authenticatorStub{user: domain.User{ID: 1, Role: "admin"}, calls: &fallbackCalls},
	)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/search", nil))

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Equal(t, 1, firstCalls)
	assert.Zero(t, fallbackCalls)
	assert.JSONEq(
		t,
		`{"error":"Unauthorized.","problems":{"authorization":"Authorization header must contain a valid Bearer token."}}`,
		response.Body.String(),
	)
}

// TestAuthenticateAPIContinuesWhenCredentialsAreAbsent verifies that missing credentials may fall through to browser
// auth.
func TestAuthenticateAPIContinuesWhenCredentialsAreAbsent(t *testing.T) {
	t.Parallel()

	fallbackCalls := 0
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := AuthenticateAPI(
		logger,
		authenticatorStub{err: auth.ErrUnauthenticated},
		authenticatorStub{user: domain.User{ID: 1, Role: "admin"}, calls: &fallbackCalls},
	)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.User(r)

		require.True(t, ok)
		assert.Equal(t, int64(1), user.ID)
		w.WriteHeader(http.StatusNoContent)
	}))

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/search", nil))

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, 1, fallbackCalls)
}

// TestAuthenticateAPIReturnsServerErrorForUnexpectedAuthFailure verifies unexpected authenticator failures remain
// internal errors.
func TestAuthenticateAPIReturnsServerErrorForUnexpectedAuthFailure(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := AuthenticateAPI(
		logger,
		authenticatorStub{err: errors.New("boom")},
	)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/search", nil))
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.JSONEq(
		t,
		`{"error":"The request could not be processed.","problems":{}}`,
		response.Body.String(),
	)
}

// TestAuthenticateAPILogsDeniedRequests verifies authentication failures are visible without exposing credentials.
func TestAuthenticateAPILogsDeniedRequests(t *testing.T) {
	t.Parallel()

	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := AuthenticateAPI(
		logger,
		authenticatorStub{err: auth.ErrInvalidCredentials},
	)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}),
	)

	request := httptest.NewRequest("GET", "/api/search?q=postgres", nil)

	request.Header.Set("Authorization", "Bearer super-secret-token")

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Contains(t, logs.String(), "authentication_denied")
	assert.Contains(t, logs.String(), "invalid_credentials")
	assert.Contains(t, logs.String(), "/api/search")
	assert.NotContains(t, logs.String(), "super-secret-token")
}

// TestAuthenticateAPILogsMissingCredentials verifies the final unauthenticated outcome is logged by unauthorized.
func TestAuthenticateAPILogsMissingCredentials(t *testing.T) {
	t.Parallel()

	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := AuthenticateAPI(
		logger,
		authenticatorStub{err: auth.ErrUnauthenticated},
	)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest("GET", "/api/search", nil))

	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Contains(t, logs.String(), "authentication_denied")
	assert.Contains(t, logs.String(), "credentials_required")
	assert.JSONEq(
		t,
		`{"error":"Unauthorized.","problems":{"authorization":"Authentication credentials are required."}}`,
		response.Body.String(),
	)
}

func TestBrowserReturnDestination(t *testing.T) {
	t.Parallel()

	t.Run("GET preserves the original URI", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodGet, "/admin/users?filter=all", nil)

		assert.Equal(t, "/admin/users?filter=all", browserReturnDestination(request))
	})

	t.Run("HEAD preserves the original URI", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodHead, "/admin/users?filter=all", nil)

		assert.Equal(t, "/admin/users?filter=all", browserReturnDestination(request))
	})

	t.Run("OIDC mutation returns to user administration", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodPost, "/admin/oidc/pending/42/reopen", nil)

		assert.Equal(t, "/admin/users", browserReturnDestination(request))
	})

	t.Run("user mutation returns to user administration", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodPost, "/admin/users/7", nil)

		assert.Equal(t, "/admin/users", browserReturnDestination(request))
	})

	t.Run("other mutation returns home", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodPost, "/admin/settings", nil)

		assert.Equal(t, "/", browserReturnDestination(request))
	})
}

func TestBrowserUnauthorizedRedirectsToLogin(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/admin/users/7", nil)

	browserUnauthorized(response, request, unauthorizedCredentialsRequired)

	require.Equal(t, http.StatusFound, response.Code)

	location, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/auth/login", location.Path)
	assert.Equal(t, "/admin/users", location.Query().Get("next"))
}
