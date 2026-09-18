package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigureBrowserAuth verifies browser authentication wiring exposes all expected operations.
func TestConfigureBrowserAuth(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{setupRequired: true}
	configured, err := ConfigureBrowserAuth(
		context.Background(),
		BrowserConfig{ModeOverride: domain.AuthModeNone},
		repository,
	)

	require.NoError(t, err)
	assert.NotNil(t, configured.Authenticator)
	assert.NotNil(t, configured.Login)
	assert.NotNil(t, configured.Callback)
	assert.NotNil(t, configured.Validate)
	assert.NotNil(t, configured.Local)
	assert.NotNil(t, configured.LocalLoginAllowed)
}

// TestBrowserAuthenticatorForSettings verifies settings select the expected authenticator implementation.
func TestBrowserAuthenticatorForSettings(t *testing.T) {
	t.Parallel()

	browser := &browserAuthenticator{none: NewNone(nil), local: NewLocal(nil, "")}

	t.Run("no authentication", func(t *testing.T) {
		authenticator, err := browser.authenticatorForSettings(
			context.Background(),
			domain.AuthenticationSettings{Mode: string(domain.AuthModeNone)},
		)

		require.NoError(t, err)
		assert.IsType(t, &None{}, authenticator)
	})

	t.Run("local", func(t *testing.T) {
		authenticator, err := browser.authenticatorForSettings(
			context.Background(),
			domain.AuthenticationSettings{Mode: string(domain.AuthModeLocal)},
		)

		require.NoError(t, err)
		assert.IsType(t, &Local{}, authenticator)
	})

	t.Run("trusted proxy", func(t *testing.T) {
		authenticator, err := browser.authenticatorForSettings(
			context.Background(),
			domain.AuthenticationSettings{
				Mode:                   string(domain.AuthModeTrustedProxy),
				TrustedUsernameHeaders: []string{"X-User"},
			},
		)

		require.NoError(t, err)
		assert.IsType(t, &TrustedProxy{}, authenticator)
	})

	t.Run("rejects unknown mode", func(t *testing.T) {
		_, err := browser.authenticatorForSettings(
			context.Background(),
			domain.AuthenticationSettings{Mode: "invalid"},
		)
		require.Error(t, err)
	})
}

// TestBrowserAuthenticatorValidatesOIDCSecrets verifies OIDC activation requires deployment-managed secrets.
func TestBrowserAuthenticatorValidatesOIDCSecrets(t *testing.T) {
	t.Parallel()

	settings := domain.AuthenticationSettings{
		Mode:         string(domain.AuthModeOIDC),
		OIDCIssuer:   "https://identity.example.com",
		OIDCClientID: "kumbuka",
	}
	browser := &browserAuthenticator{}

	validation, ok := errors.AsType[*domain.ValidationError](browser.validateSettings(settings))
	require.True(t, ok)
	assert.Equal(t, "oidc_client_secret", validation.Fields[0].Field)
	assert.Equal(t, "Configure KUMBUKA__OIDC_CLIENT_SECRET before enabling OIDC.", validation.UserMessage())

	browser.oidcConfig.ClientSecret = "client-secret"

	validation, ok = errors.AsType[*domain.ValidationError](browser.validateSettings(settings))
	require.True(t, ok)
	assert.Equal(t, "oidc_session_secret", validation.Fields[0].Field)
	assert.Equal(t, "Configure KUMBUKA__OIDC_SESSION_SECRET with at least 32 characters before enabling OIDC.", validation.UserMessage())
}

// TestBrowserValidationRequiresAdministratorGroupSources verifies administrator elevation requires a matching group source.
func TestBrowserValidationRequiresAdministratorGroupSources(t *testing.T) {
	t.Parallel()
	browser := &browserAuthenticator{oidcConfig: OIDCConfig{ClientSecret: "secret", SessionSecret: "0123456789abcdef0123456789abcdef"}}
	assert.Error(t, browser.validateSettings(domain.AuthenticationSettings{Mode: "oidc", OIDCIssuer: "https://example.test", OIDCClientID: "kumbuka", OIDCAdminGroup: "/admins"}))
	assert.Error(t, browser.validateSettings(domain.AuthenticationSettings{Mode: "trusted-proxy", TrustedUsernameHeaders: []string{"X-User"}, TrustedAdminGroup: "/admins"}))
}
