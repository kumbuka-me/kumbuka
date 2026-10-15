package bootstrap

import (
	"context"
	"net/http"
	"testing"

	appkitproxy "github.com/containeroo/appkit/auth/proxy"
	"time"

	appkitauth "github.com/containeroo/appkit/auth"
	appkitconfig "github.com/containeroo/appkit/config"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBrowserAuthConfigMapsDeploymentSettings(t *testing.T) {
	t.Parallel()

	allowRegistration := true
	authMode := appkitauth.ModeOIDC
	cfg := flags.Config{
		Server: appkitconfig.Server{PublicURL: "https://wiki.example"},
		Authentication: appkitconfig.Authentication{
			ModeOverride:              &authMode,
			LocalLogin:                true,
			AllowRegistrationOverride: &allowRegistration,
		},
		Proxy: appkitconfig.Proxy{
			TrustedCIDRs:       []string{"127.0.0.1/32"},
			UsernameHeaders:    []string{"X-User", "Remote-User"},
			EmailHeaders:       []string{"X-Email"},
			DisplayNameHeaders: []string{"X-Name"},
			GroupHeaders:       []string{"X-Groups"},
			AdminGroup:         "admins",
		},
		OIDC: appkitconfig.OIDC{
			ClientID:      "kumbuka",
			ClientSecret:  "client-secret",
			Issuer:        "https://id.example",
			SessionSecret: "01234567890123456789012345678901",
			GroupClaim:    "roles",
			AdminGroup:    "wiki-admins",
		},
	}

	// Only values recorded in cfg.Overrides should be considered deployment overrides.
	got := newBrowserAuthConfig(cfg)
	want := auth.BrowserConfig{
		ModeOverride:    domain.AuthModeOIDC,
		ProxyOverrides:  appkitproxy.Layer{},
		ProxyAdminGroup: "admins",
		OIDC: auth.OIDCConfig{
			ClientID:      "kumbuka",
			ClientSecret:  "client-secret",
			Issuer:        "https://id.example",
			SessionSecret: "01234567890123456789012345678901",
			PublicURL:     "https://wiki.example",
			GroupClaim:    "roles",
			AdminGroup:    "wiki-admins",
		},
		LocalLoginEnabled:             true,
		AllowUserRegistrationOverride: &allowRegistration,
	}

	require.NotNil(t, got.AllowUserRegistrationOverride)
	assert.Equal(t, want, got)
}

// TestNewBrowserAuthConfigPreservesExplicitProxyOverrides verifies only selected flags reach appkit.
func TestNewBrowserAuthConfigPreservesExplicitProxyOverrides(t *testing.T) {
	t.Parallel()
	cfg, err := flags.Parse([]string{
		"--auth-mode=proxy", "--trusted-proxy-cidrs=*", "--trusted-username-headers=",
		"--trusted-email-headers=X-Auth-Request-Email",
	}, "test")
	require.NoError(t, err)
	got := newBrowserAuthConfig(cfg)
	require.NotNil(t, got.ProxyOverrides.TrustedCIDRs)
	require.NotNil(t, got.ProxyOverrides.UsernameHeaders)
	require.NotNil(t, got.ProxyOverrides.EmailHeaders)
	assert.Equal(t, []string{"*"}, *got.ProxyOverrides.TrustedCIDRs)
	assert.Empty(t, *got.ProxyOverrides.UsernameHeaders)
	assert.Equal(t, []string{"X-Auth-Request-Email"}, *got.ProxyOverrides.EmailHeaders)
	assert.Nil(t, got.ProxyOverrides.GroupHeaders)
}

func TestAuthenticatedPluginRequest(t *testing.T) {
	t.Parallel()

	t.Run("without authenticated user", func(t *testing.T) {
		t.Parallel()

		assert.False(t, authenticatedPluginRequest(context.Background()))
	})

	t.Run("with authenticated user", func(t *testing.T) {
		t.Parallel()

		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		assert.NoError(t, err)
		request = auth.WithUser(request, domain.User{ID: 42})

		assert.True(t, authenticatedPluginRequest(request.Context()))
	})
}

// blockingApplicationWorker records cancellation and waits for explicit release before returning.
type blockingApplicationWorker struct {
	// started closes after Run begins.
	started chan struct{}
	// canceled closes after Run observes application cancellation.
	canceled chan struct{}
	// release allows Run to return after cancellation.
	release chan struct{}
}

// Run blocks until the application cancels the worker and the test releases it.
func (w *blockingApplicationWorker) Run(ctx context.Context) {
	close(w.started)
	<-ctx.Done()
	close(w.canceled)
	<-w.release
}

func TestApplicationCloseCancelsAndJoinsBackgroundWorkers(t *testing.T) {
	t.Parallel()

	worker := &blockingApplicationWorker{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	application := &Application{backgroundWorkers: []applicationWorker{worker}}
	application.Start(context.Background())

	select {
	case <-worker.started:
	case <-time.After(time.Second):
		t.Fatal("background worker did not start")
	}

	closed := make(chan struct{})
	go func() {
		application.Close(nil)
		close(closed)
	}()

	select {
	case <-worker.canceled:
	case <-time.After(time.Second):
		t.Fatal("background worker was not canceled")
	}

	select {
	case <-closed:
		t.Fatal("application closed before background worker returned")
	default:
	}

	close(worker.release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("application did not finish closing after background worker returned")
	}
}
