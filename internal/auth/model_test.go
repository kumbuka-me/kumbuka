package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureBrowserAuth(t *testing.T) {
	t.Parallel()

	configured, err := ConfigureBrowserAuth(
		context.Background(),
		BrowserConfig{ModeOverride: AuthModeNone},
		nil,
	)

	require.NoError(t, err)
	assert.NotNil(t, configured.Authenticator)
	assert.NotNil(t, configured.Login)
	assert.NotNil(t, configured.Callback)
	assert.NotNil(t, configured.Validate)
	assert.NotNil(t, configured.Local)
	assert.NotNil(t, configured.LocalLoginAllowed)
}

func TestBrowserAuthenticatorForSettings(t *testing.T) {
	t.Parallel()

	browser := &browserAuthenticator{none: NewNone(nil), local: NewLocal(nil, "")}

	t.Run("no authentication", func(t *testing.T) {
		authenticator, err := browser.authenticatorForSettings(
			context.Background(),
			domain.AuthenticationSettings{Mode: string(AuthModeNone)},
		)

		require.NoError(t, err)
		assert.IsType(t, &None{}, authenticator)
	})

	t.Run("local", func(t *testing.T) {
		authenticator, err := browser.authenticatorForSettings(
			context.Background(),
			domain.AuthenticationSettings{Mode: string(AuthModeLocal)},
		)

		require.NoError(t, err)
		assert.IsType(t, &Local{}, authenticator)
	})

	t.Run("trusted proxy", func(t *testing.T) {
		authenticator, err := browser.authenticatorForSettings(
			context.Background(),
			domain.AuthenticationSettings{
				Mode:                   string(AuthModeTrustedProxy),
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

func TestBrowserAuthenticatorValidatesOIDCSecrets(t *testing.T) {
	t.Parallel()

	settings := domain.AuthenticationSettings{
		Mode:         string(AuthModeOIDC),
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

func TestBrowserValidationRequiresAdministratorGroupSources(t *testing.T) {
	t.Parallel()
	browser := &browserAuthenticator{oidcConfig: OIDCConfig{ClientSecret: "secret", SessionSecret: "0123456789abcdef0123456789abcdef"}}
	assert.Error(t, browser.validateSettings(domain.AuthenticationSettings{Mode: "oidc", OIDCIssuer: "https://example.test", OIDCClientID: "kumbuka", OIDCAdminGroup: "/admins"}))
	assert.Error(t, browser.validateSettings(domain.AuthenticationSettings{Mode: "trusted-proxy", TrustedUsernameHeaders: []string{"X-User"}, TrustedAdminGroup: "/admins"}))
}
