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
	authenticator := NewTrustedProxy(repository, TrustedProxyHeaders{
		Username:   []string{"X-User"},
		Groups:     []string{"X-Groups"},
		AdminGroup: "/kumbuka-admins",
	}, trustedTestProxy())
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
	authenticator := NewTrustedProxy(
		repository,
		TrustedProxyHeaders{Username: []string{"X-User"}},
		trustedTestProxy(),
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
	authenticator := NewTrustedProxy(repository, TrustedProxyHeaders{
		Username: []string{"X-Primary-User", "X-Secondary-User"},
	}, trustedTestProxy())
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
	authenticator := NewTrustedProxy(repository, TrustedProxyHeaders{
		Username: []string{"X-User"},
	}, trustedTestProxy())
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
	authenticator := NewTrustedProxy(repository, TrustedProxyHeaders{Username: []string{"X-User"}}, nil)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-User", "admin")
	_, err := authenticator.Authenticate(request)
	require.ErrorIs(t, err, ErrUnauthenticated)
	assert.Zero(t, repository.calls)
}

// trustedTestProxy trusts only the fixture's documented example network.
func trustedTestProxy() appkitproxy.TrustFunc {
	trust, err := appkitproxy.TrustCIDRs([]string{"192.0.2.0/24"})
	if err != nil {
		panic(err)
	}
	return trust
}

func TestTrustedProxyClearsReturnedExternalAdminStatus(t *testing.T) {
	t.Parallel()
	repository := &trustedProxyRepositoryStub{user: domain.User{ID: 7, Role: "viewer", Enabled: true, ExternalAdmin: true}}
	authenticator := NewTrustedProxy(repository, TrustedProxyHeaders{Username: []string{"X-User"}, Groups: []string{"X-Groups"}, AdminGroup: "/admins"}, trustedTestProxy())
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("X-User", "example")
	request.Header.Set("X-Groups", "/readers")
	user, err := authenticator.Authenticate(request)
	require.NoError(t, err)
	assert.False(t, user.ExternalAdmin)
	assert.False(t, repository.admin)
	assert.Equal(t, domain.UserRoleViewer, user.Role)
}
