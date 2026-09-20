package auth

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trustedProxyRepositoryStub struct {
	user   domain.User
	method string
	admin  bool
}

func (r *trustedProxyRepositoryStub) TrustedProxyUser(context.Context, string, string, string) (domain.User, error) {
	return r.user, nil
}

func (r *trustedProxyRepositoryStub) SetExternalAdminStatus(_ context.Context, _ int64, method string, admin bool) error {
	r.method = method
	r.admin = admin

	return nil
}

func TestTrustedProxyExternalAdministrator(t *testing.T) {
	t.Parallel()

	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Role: "viewer", Enabled: true}}
	authenticator := NewTrustedProxy(repository, TrustedProxyHeaders{
		Username:   []string{"X-User"},
		Groups:     []string{"X-Groups"},
		AdminGroup: "/kumbuka-admins",
	})
	request := httptest.NewRequest("GET", "/", nil)

	request.Header.Set("X-User", "daniel")
	request.Header.Set("X-Groups", "/family, /kumbuka-admins")

	user, err := authenticator.Authenticate(request)

	require.NoError(t, err)
	assert.Equal(t, "admin", user.Role)
	assert.True(t, user.ExternalAdmin)
	assert.Equal(t, "trusted-proxy", repository.method)
	assert.True(t, repository.admin)
}

func TestTrustedProxyDisabledAccount(t *testing.T) {
	t.Parallel()

	authenticator := NewTrustedProxy(
		&trustedProxyRepositoryStub{user: domain.User{ID: 7, Enabled: false}},
		TrustedProxyHeaders{Username: []string{"X-User"}},
	)
	request := httptest.NewRequest("GET", "/", nil)

	request.Header.Set("X-User", "daniel")

	_, err := authenticator.Authenticate(request)

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestFirstHeader(t *testing.T) {
	t.Parallel()

	t.Run("uses the first populated configured header", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("GET", "/", nil)

		request.Header.Set("X-Secondary-User", "daniel")
		request.Header.Set("X-Tertiary-User", "ignored")

		assert.Equal(t, "daniel", firstHeader(request, []string{
			"X-Primary-User",
			"X-Secondary-User",
			"X-Tertiary-User",
		}))
	})

	t.Run("trims surrounding whitespace", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("GET", "/", nil)

		request.Header.Set("X-User", "  daniel  ")

		assert.Equal(t, "daniel", firstHeader(request, []string{"X-User"}))
	})

	t.Run("returns empty when no configured header is populated", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("GET", "/", nil)

		assert.Empty(t, firstHeader(request, []string{"X-User"}))
	})
}

func TestTrustedProxyClearsReturnedExternalAdminStatus(t *testing.T) {
	t.Parallel()
	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Role: "viewer", Enabled: true, ExternalAdmin: true}}
	authenticator := NewTrustedProxy(repository, TrustedProxyHeaders{Username: []string{"X-User"}, Groups: []string{"X-Groups"}, AdminGroup: "/admins"})
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("X-User", "example")
	request.Header.Set("X-Groups", "/readers")
	user, err := authenticator.Authenticate(request)
	require.NoError(t, err)
	assert.False(t, user.ExternalAdmin)
	assert.False(t, repository.admin)
	assert.Equal(t, "viewer", user.Role)
}
