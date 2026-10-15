package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	appkitproxy "github.com/containeroo/appkit/auth/proxy"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trustedProxyRepositoryStub provides controllable trusted proxy repository behavior for tests.
type trustedProxyRepositoryStub struct {
	// user records the user observed by the test double.
	user domain.User
	// adminObserved records whether an external admin assertion source was configured.
	adminObserved bool
	// admin records the current asserted external administrator state.
	admin bool
	// username is the effective username passed to provisioning.
	username string
	// calls counts attempted local identity resolutions.
	calls int
}

func (r *trustedProxyRepositoryStub) Login(_ context.Context, username, _, _ string, adminObserved, admin bool) (domain.User, error) {
	r.username = username
	r.calls++
	r.adminObserved = adminObserved
	r.admin = admin
	return r.user, nil
}

func TestTrustedProxyExternalAdministrator(t *testing.T) {
	t.Parallel()

	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Role: "viewer", Enabled: true}}
	authenticator := newTestTrustedProxy(t, repository, proxyTestHeaders{
		Username:   []string{"X-User"},
		Groups:     []string{"X-Groups"},
		AdminGroup: "/kumbuka-admins",
	})
	request := httptest.NewRequest("GET", "/", nil)

	request.Header.Set("X-User", "daniel")
	request.Header.Set("X-Groups", "/family, /kumbuka-admins")

	user, err := authenticator.Authenticate(request)

	require.NoError(t, err)
	assert.Equal(t, domain.UserRoleAdmin, user.Role)
	assert.True(t, user.ExternalAdmin)
	assert.True(t, repository.adminObserved)
	assert.True(t, repository.admin)
}

func TestTrustedProxyDisabledAccount(t *testing.T) {
	t.Parallel()

	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Enabled: false}}
	authenticator := newTestTrustedProxy(
		t, repository,
		proxyTestHeaders{Username: []string{"X-User"}},
	)
	request := httptest.NewRequest("GET", "/", nil)

	request.Header.Set("X-User", "daniel")

	_, err := authenticator.Authenticate(request)

	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

// TestTrustedProxyHeaderPriority verifies ordered header fallback in the shared proxy adapter.
func TestTrustedProxyHeaderPriority(t *testing.T) {
	t.Parallel()
	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Enabled: true}}
	authenticator := newTestTrustedProxy(t, repository, proxyTestHeaders{
		Username: []string{"X-Primary-User", "X-Secondary-User"},
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Secondary-User", "  daniel  ")
	_, err := authenticator.Authenticate(request)
	require.NoError(t, err)
	assert.Equal(t, "daniel", repository.username)
	request.Header.Set("X-Primary-User", "alice")
	_, err = authenticator.Authenticate(request)
	require.NoError(t, err)
	assert.Equal(t, "alice", repository.username)
}

// TestTrustedProxyRejectsUntrustedPeer verifies untrusted clients cannot assert identities.
func TestTrustedProxyRejectsUntrustedPeer(t *testing.T) {
	t.Parallel()
	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Enabled: true}}
	authenticator := newTestTrustedProxy(t, repository, proxyTestHeaders{
		Username: []string{"X-User"},
	})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "198.51.100.1:50000"
	request.Header.Set("X-User", "admin")
	_, err := authenticator.Authenticate(request)
	require.ErrorIs(t, err, ErrUnauthenticated)
	assert.Zero(t, repository.calls)

	// Forged forwarded headers must not override the immediate TCP peer.
	request.Header.Set("X-Forwarded-For", "192.0.2.1")
	_, err = authenticator.Authenticate(request)
	require.ErrorIs(t, err, ErrUnauthenticated)
	assert.Zero(t, repository.calls)
}

// TestTrustedProxyRequiresExplicitTrust verifies a missing allowlist cannot authenticate anyone.
func TestTrustedProxyRequiresExplicitTrust(t *testing.T) {
	t.Parallel()
	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Enabled: true}}
	_, err := NewTrustedProxy(repository, appkitproxy.Config{UsernameHeaders: []string{"X-User"}}, "")
	require.ErrorContains(t, err, "trusted-proxy-cidrs")
	assert.Zero(t, repository.calls)
}

// proxyTestHeaders contains test-specific identity header choices and admin mapping.
type proxyTestHeaders struct {
	Username []string
	Email    []string
	// DisplayName is the optional display-name assertion candidate.
	DisplayName []string
	Groups      []string
	AdminGroup  string
}

// newTestTrustedProxy creates an appkit-backed authenticator with a trusted test ingress.
func newTestTrustedProxy(t *testing.T, repository *trustedProxyRepositoryStub, headers proxyTestHeaders) *TrustedProxy {
	t.Helper()
	provider, err := NewTrustedProxy(repository, appkitproxy.Config{
		TrustedCIDRs: []string{"192.0.2.0/24"}, UsernameHeaders: headers.Username,
		EmailHeaders: headers.Email, DisplayNameHeaders: headers.DisplayName,
		GroupHeaders: headers.Groups,
	}, headers.AdminGroup)
	require.NoError(t, err)
	return provider
}

func TestTrustedProxyClearsReturnedExternalAdminStatus(t *testing.T) {
	t.Parallel()
	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Role: "viewer", Enabled: true, ExternalAdmin: true}}
	authenticator := newTestTrustedProxy(t, repository, proxyTestHeaders{Username: []string{"X-User"}, Groups: []string{"X-Groups"}, AdminGroup: "/admins"})
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("X-User", "example")
	request.Header.Set("X-Groups", "/readers")
	user, err := authenticator.Authenticate(request)
	require.NoError(t, err)
	assert.False(t, user.ExternalAdmin)
	assert.False(t, repository.admin)
	assert.Equal(t, domain.UserRoleViewer, user.Role)
}
