package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type setupBrowserRepository struct {
	browserRepository
	settings               domain.ApplicationSettings
	setupRequired          bool
	localAdminCredential   bool
	localCredentialChecked bool
}

func (r *setupBrowserRepository) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return r.settings, nil
}

func (r *setupBrowserRepository) SetupRequired(context.Context) (bool, error) {
	return r.setupRequired, nil
}

func (r *setupBrowserRepository) HasLocalAdministratorCredential(context.Context) (bool, error) {
	r.localCredentialChecked = true
	return r.localAdminCredential, nil
}

func TestConfigureBrowserAuthAllowsSetupWithStaleLocalMode(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{
		settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{Mode: string(AuthModeLocal)},
		},
		setupRequired: true,
	}

	configured, err := ConfigureBrowserAuth(context.Background(), BrowserConfig{}, repository)

	require.NoError(t, err)
	assert.NotNil(t, configured.Authenticator)
	assert.False(t, repository.localCredentialChecked)
}

func TestBrowserLoginRedirectsSetupWithStaleLocalMode(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{
		settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{Mode: string(AuthModeLocal)},
		},
		setupRequired: true,
	}
	browser := &browserAuthenticator{repository: repository}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/login", nil)

	browser.login(response, request)

	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/setup", response.Header().Get("Location"))
}

func TestBrowserValidationStillRequiresLocalAdministratorAfterSetup(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{setupRequired: false}
	browser := &browserAuthenticator{repository: repository}

	err := browser.validate(context.Background(), domain.AuthenticationSettings{Mode: string(AuthModeLocal)})

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "auth_mode", validation.Fields[0].Field)
	assert.Equal(t, "Local authentication requires an administrator with a local password.", validation.UserMessage())
	assert.True(t, repository.localCredentialChecked)
}

func TestBrowserCurrentSettingsOverlaysRuntimeManagedFields(t *testing.T) {
	t.Parallel()

	t.Run("OIDC", func(t *testing.T) {
		t.Parallel()

		repository := &setupBrowserRepository{settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{
				Mode:           string(AuthModeLocal),
				OIDCIssuer:     "https://stored.example.test",
				OIDCClientID:   "stored-client",
				OIDCGroupClaim: "groups",
				OIDCAdminGroup: "/admins",
			},
		}}
		browser := &browserAuthenticator{
			repository:   repository,
			modeOverride: AuthModeOIDC,
			oidcConfig: OIDCConfig{
				Issuer:   "https://runtime.example.test",
				ClientID: "runtime-client",
			},
		}

		settings, err := browser.currentSettings(context.Background())

		require.NoError(t, err)
		assert.Equal(t, string(AuthModeOIDC), settings.Mode)
		assert.Equal(t, "https://runtime.example.test", settings.OIDCIssuer)
		assert.Equal(t, "runtime-client", settings.OIDCClientID)
		assert.Equal(t, "groups", settings.OIDCGroupClaim)
		assert.Equal(t, "/admins", settings.OIDCAdminGroup)
	})

	t.Run("trusted proxy", func(t *testing.T) {
		t.Parallel()

		repository := &setupBrowserRepository{settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{
				Mode:                      string(AuthModeLocal),
				TrustedUsernameHeaders:    []string{"Stored-User"},
				TrustedEmailHeaders:       []string{"Stored-Email"},
				TrustedDisplayNameHeaders: []string{"Stored-Name"},
				TrustedGroupHeaders:       []string{"X-Groups"},
				TrustedAdminGroup:         "admins",
			},
		}}
		browser := &browserAuthenticator{
			repository:   repository,
			modeOverride: AuthModeTrustedProxy,
			trustedProxy: TrustedProxyHeaders{
				Username:    []string{"Runtime-User"},
				Email:       []string{"Runtime-Email"},
				DisplayName: []string{"Runtime-Name"},
			},
		}

		settings, err := browser.currentSettings(context.Background())

		require.NoError(t, err)
		assert.Equal(t, string(AuthModeTrustedProxy), settings.Mode)
		assert.Equal(t, []string{"Runtime-User"}, settings.TrustedUsernameHeaders)
		assert.Equal(t, []string{"Runtime-Email"}, settings.TrustedEmailHeaders)
		assert.Equal(t, []string{"Runtime-Name"}, settings.TrustedDisplayNameHeaders)
		assert.Equal(t, []string{"X-Groups"}, settings.TrustedGroupHeaders)
		assert.Equal(t, "admins", settings.TrustedAdminGroup)
	})
}
