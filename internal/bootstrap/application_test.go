package bootstrap

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBrowserAuthConfigMapsDeploymentSettings(t *testing.T) {
	t.Parallel()

	allowRegistration := true
	cfg := flags.Config{
		AuthModeOverride:              domain.AuthModeOIDC,
		TrustedUsernameHeaders:        []string{"X-User", "Remote-User"},
		TrustedEmailHeaders:           []string{"X-Email"},
		TrustedDisplayNameHeaders:     []string{"X-Name"},
		TrustedGroupHeaders:           []string{"X-Groups"},
		TrustedAdminGroup:             "admins",
		OIDCClientID:                  "kumbuka",
		OIDCClientSecret:              "client-secret",
		OIDCIssuer:                    "https://id.example",
		OIDCSessionSecret:             "01234567890123456789012345678901",
		PublicURL:                     "https://wiki.example",
		OIDCGroupClaim:                "roles",
		OIDCAdminGroup:                "wiki-admins",
		LocalLogin:                    true,
		AllowUserRegistrationOverride: &allowRegistration,
	}

	got := newBrowserAuthConfig(cfg)
	want := auth.BrowserConfig{
		ModeOverride: domain.AuthModeOIDC,
		TrustedProxy: auth.TrustedProxyHeaders{
			Username:    []string{"X-User", "Remote-User"},
			Email:       []string{"X-Email"},
			DisplayName: []string{"X-Name"},
			Groups:      []string{"X-Groups"},
			AdminGroup:  "admins",
		},
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
