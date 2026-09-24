package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// oidcRepositoryStub provides controllable OIDC repository behavior for tests.
type oidcRepositoryStub struct {
	// user records the user observed by the test double.
	user domain.User
}

func (r *oidcRepositoryStub) LoginOIDCUser(context.Context, string, string, string, string, string) (domain.User, error) {
	return r.user, nil
}

func (r *oidcRepositoryStub) OIDCUser(context.Context, string, string) (domain.User, error) {
	return r.user, nil
}

func (*oidcRepositoryStub) SyncOIDCGroups(context.Context, int64, []string, []domain.OIDCGroupMapping, bool) error {
	return nil
}

func (*oidcRepositoryStub) SetExternalAdminStatus(context.Context, int64, domain.AuthMode, bool) error {
	return nil
}

func TestOIDCAuthenticateExternalAdministrator(t *testing.T) {
	t.Parallel()

	const issuer = "https://identity.example.com/realms/kumbuka"
	repository := &oidcRepositoryStub{user: domain.User{
		ID:             7,
		Role:           "viewer",
		Enabled:        true,
		ExternalAdmin:  true,
		SessionVersion: 4,
	}}
	authenticator := &OIDC{
		repository: repository,
		secret:     []byte("0123456789abcdef0123456789abcdef"),
		issuer:     issuer,
		adminGroup: "/kumbuka-admins",
	}
	request := oidcSessionRequest(t, authenticator, session{
		Issuer: issuer, Subject: "user-123", Expires: time.Now().Add(time.Hour).Unix(), Version: 4,
	})

	user, err := authenticator.Authenticate(request)

	require.NoError(t, err)
	assert.Equal(t, domain.UserRoleAdmin, user.Role)

	authenticator.adminGroup = ""
	user, err = authenticator.Authenticate(request)

	require.NoError(t, err)
	assert.Equal(t, domain.UserRoleViewer, user.Role, "a removed administrator-group setting must stop stale elevation")
}

func TestOIDCAuthenticateRejectsRevokedSession(t *testing.T) {
	t.Parallel()

	const issuer = "https://identity.example.com/realms/kumbuka"
	authenticator := &OIDC{
		repository: &oidcRepositoryStub{user: domain.User{Enabled: true, SessionVersion: 5}},
		secret:     []byte("0123456789abcdef0123456789abcdef"),
		issuer:     issuer,
	}
	request := oidcSessionRequest(t, authenticator, session{
		Issuer: issuer, Subject: "user-123", Expires: time.Now().Add(time.Hour).Unix(), Version: 4,
	})

	_, err := authenticator.Authenticate(request)

	assert.ErrorIs(t, err, ErrUnauthenticated)
}

func TestOIDCAuthenticateRejectsUnboundSessions(t *testing.T) {
	t.Parallel()

	authenticator := &OIDC{
		secret: []byte("0123456789abcdef0123456789abcdef"),
		issuer: "https://identity.example.com/realms/kumbuka",
	}

	t.Run("rejects session from another issuer", func(t *testing.T) {
		t.Parallel()

		request := oidcSessionRequest(t, authenticator, session{
			Issuer:  "https://identity.example.com/realms/other",
			Subject: "user-123",
			Expires: time.Now().Add(time.Hour).Unix(),
		})

		_, err := authenticator.Authenticate(request)

		assert.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("rejects session without subject", func(t *testing.T) {
		t.Parallel()

		request := oidcSessionRequest(t, authenticator, session{
			Issuer:  authenticator.issuer,
			Expires: time.Now().Add(time.Hour).Unix(),
		})

		_, err := authenticator.Authenticate(request)

		assert.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("rejects session without version", func(t *testing.T) {
		t.Parallel()

		request := oidcSessionRequest(t, authenticator, session{
			Issuer:  authenticator.issuer,
			Subject: "user-123",
			Expires: time.Now().Add(time.Hour).Unix(),
		})

		_, err := authenticator.Authenticate(request)

		assert.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("rejects expired session", func(t *testing.T) {
		t.Parallel()

		request := oidcSessionRequest(t, authenticator, session{
			Issuer:  authenticator.issuer,
			Subject: "user-123",
			Expires: time.Now().Add(-time.Hour).Unix(),
		})

		_, err := authenticator.Authenticate(request)

		assert.ErrorIs(t, err, ErrUnauthenticated)
	})
}

// oidcSessionRequest creates a request carrying one signed Kumbuka session cookie.
func oidcSessionRequest(t *testing.T, authenticator *OIDC, value any) *http.Request {
	t.Helper()

	response := httptest.NewRecorder()

	authenticator.setCookie(response, "kumbuka_session", value, 3600)

	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)

	request := httptest.NewRequest("GET", "/", nil)

	request.AddCookie(cookies[0])
	return request
}

func TestOIDCGroupValues(t *testing.T) {
	t.Parallel()

	t.Run("accepts string array", func(t *testing.T) {
		t.Parallel()

		groups, err := oidcGroupValues([]byte(`["/admins"," /family ","/admins"]`))

		assert.NoError(t, err)
		assert.Equal(t, []string{"/admins", "/family"}, groups)
	})

	t.Run("accepts one string", func(t *testing.T) {
		t.Parallel()

		groups, err := oidcGroupValues([]byte(`"/admins"`))

		assert.NoError(t, err)
		assert.Equal(t, []string{"/admins"}, groups)
	})

	t.Run("accepts missing claim", func(t *testing.T) {
		t.Parallel()

		groups, err := oidcGroupValues(nil)

		assert.NoError(t, err)
		assert.Empty(t, groups)
	})

	t.Run("rejects non-string values", func(t *testing.T) {
		t.Parallel()

		_, err := oidcGroupValues([]byte(`{"name":"admins"}`))

		assert.Error(t, err)
	})
}

func TestOIDCCallbackRejectsInvalidState(t *testing.T) {
	t.Parallel()

	t.Run("missing state and verifier", func(t *testing.T) {
		t.Parallel()

		authenticator := &OIDC{secret: []byte("0123456789abcdef0123456789abcdef")}
		saved := loginState{Expires: time.Now().Add(time.Minute).Unix()}
		response := httptest.NewRecorder()
		authenticator.setCookie(response, "kumbuka_state", saved, 600)
		cookies := response.Result().Cookies()
		require.Len(t, cookies, 1)
		request := httptest.NewRequest("GET", "/auth/callback?state="+saved.State, nil)
		request.AddCookie(cookies[0])
		result := httptest.NewRecorder()

		authenticator.callback(result, request)

		assert.Equal(t, http.StatusBadRequest, result.Code)
	})

	t.Run("missing verifier", func(t *testing.T) {
		t.Parallel()

		authenticator := &OIDC{secret: []byte("0123456789abcdef0123456789abcdef")}
		saved := loginState{State: "state", Expires: time.Now().Add(time.Minute).Unix()}
		response := httptest.NewRecorder()
		authenticator.setCookie(response, "kumbuka_state", saved, 600)
		cookies := response.Result().Cookies()
		require.Len(t, cookies, 1)
		request := httptest.NewRequest("GET", "/auth/callback?state="+saved.State, nil)
		request.AddCookie(cookies[0])
		result := httptest.NewRecorder()

		authenticator.callback(result, request)

		assert.Equal(t, http.StatusBadRequest, result.Code)
	})

	t.Run("expired state", func(t *testing.T) {
		t.Parallel()

		authenticator := &OIDC{secret: []byte("0123456789abcdef0123456789abcdef")}
		saved := loginState{State: "state", Verifier: "verifier", Expires: time.Now().Unix()}
		response := httptest.NewRecorder()
		authenticator.setCookie(response, "kumbuka_state", saved, 600)
		cookies := response.Result().Cookies()
		require.Len(t, cookies, 1)
		request := httptest.NewRequest("GET", "/auth/callback?state="+saved.State, nil)
		request.AddCookie(cookies[0])
		result := httptest.NewRecorder()

		authenticator.callback(result, request)

		assert.Equal(t, http.StatusBadRequest, result.Code)
	})
}
