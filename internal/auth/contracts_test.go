package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Keep every implementation of the shared authentication boundary checked.
var (
	_ Authenticator = (*Bearer)(nil)
	_ Authenticator = (*Local)(nil)
	_ Authenticator = (*OIDC)(nil)
	_ Authenticator = (*None)(nil)
	_ Authenticator = (*TrustedProxy)(nil)
	_ Authenticator = (*browserAuthenticator)(nil)
)

type authenticationContractRepository struct {
	localRepository
	oidcRepository
	user domain.User
}

func (s authenticationContractRepository) UserByToken(context.Context, string) (domain.User, error) {
	return s.user, nil
}

func (s authenticationContractRepository) LocalUserBySession(context.Context, string) (domain.User, error) {
	return s.user, nil
}

func (s authenticationContractRepository) LocalCredential(context.Context, string) (domain.User, string, error) {
	return s.user, "", nil
}

func (s authenticationContractRepository) OIDCUser(context.Context, string, string) (domain.User, error) {
	return s.user, nil
}

func TestAuthenticationEnabledAccountContract(t *testing.T) {
	t.Parallel()

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		repository := authenticationContractRepository{user: domain.User{ID: 7, Enabled: false, SessionVersion: 1}}
		local := NewLocal(repository, "https://example.test")
		oidc := &OIDC{repository: repository, issuer: "https://identity.example.test", secret: []byte("0123456789abcdef0123456789abcdef")}
		request := httptest.NewRequest("GET", "/", nil)
		request.Header.Set("Authorization", "Bearer token")
		request.AddCookie(&http.Cookie{Name: localSessionCookie, Value: "token"})
		recorder := httptest.NewRecorder()
		oidc.setCookie(recorder, "kumbuka_session", session{Issuer: oidc.issuer, Subject: "subject", Version: 1, Expires: time.Now().Add(time.Hour).Unix()}, 3600)
		cookies := recorder.Result().Cookies()
		require.Len(t, cookies, 1)
		request.AddCookie(cookies[0])
		t.Run("bearer", func(t *testing.T) {
			user, err := NewBearer(repository).Authenticate(request)

			assert.ErrorIs(t, err, ErrInvalidCredentials)
			assert.Zero(t, user.ID)
		})

		t.Run("local", func(t *testing.T) {
			user, err := local.Authenticate(request)

			assert.ErrorIs(t, err, ErrUnauthenticated)
			assert.Zero(t, user.ID)
		})

		t.Run("oidc", func(t *testing.T) {
			user, err := oidc.Authenticate(request)

			assert.ErrorIs(t, err, ErrUnauthenticated)
			assert.Zero(t, user.ID)
		})

		t.Run("local sign-in", func(t *testing.T) {
			_, _, err := local.SignIn(context.Background(), "example", "password")
			assert.ErrorIs(t, err, ErrInvalidCredentials)
		})

		t.Run("local password change", func(t *testing.T) {
			_, err := local.ChangePassword(context.Background(), 7, "example", "current", "new")
			assert.ErrorIs(t, err, ErrInvalidCredentials)
		})
	})

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()

		repository := authenticationContractRepository{user: domain.User{ID: 7, Enabled: true, SessionVersion: 1}}
		local := NewLocal(repository, "https://example.test")
		oidc := &OIDC{repository: repository, issuer: "https://identity.example.test", secret: []byte("0123456789abcdef0123456789abcdef")}
		request := httptest.NewRequest("GET", "/", nil)
		request.Header.Set("Authorization", "Bearer token")
		request.AddCookie(&http.Cookie{Name: localSessionCookie, Value: "token"})
		recorder := httptest.NewRecorder()
		oidc.setCookie(recorder, "kumbuka_session", session{Issuer: oidc.issuer, Subject: "subject", Version: 1, Expires: time.Now().Add(time.Hour).Unix()}, 3600)
		cookies := recorder.Result().Cookies()
		require.Len(t, cookies, 1)
		request.AddCookie(cookies[0])
		t.Run("bearer", func(t *testing.T) {
			user, err := NewBearer(repository).Authenticate(request)

			require.NoError(t, err)
			assert.Equal(t, int64(7), user.ID)
		})

		t.Run("local", func(t *testing.T) {
			user, err := local.Authenticate(request)

			require.NoError(t, err)
			assert.Equal(t, int64(7), user.ID)
		})

		t.Run("oidc", func(t *testing.T) {
			user, err := oidc.Authenticate(request)

			require.NoError(t, err)
			assert.Equal(t, int64(7), user.ID)
		})
	})
}
