package bootstrap

import (
	"context"
	"net/http"
	"testing"

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
