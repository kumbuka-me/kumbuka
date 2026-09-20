package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type setupBrowserRepository struct {
	browserRepository
	settings               domain.ApplicationSettings
	setupRequired          bool
	localAdminCredential   bool
	localCredentialChecked bool
	oidcMappingsChecked    bool
	oidcMappingsErr        error
	sessionUser            domain.User
	sessionHash            string
}

// ApplicationSettings returns configured application settings for browser-auth tests.
func (r *setupBrowserRepository) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return r.settings, nil
}

// SetupRequired returns the configured setup state for browser-auth tests.
func (r *setupBrowserRepository) SetupRequired(context.Context) (bool, error) {
	return r.setupRequired, nil
}

// HasLocalAdministratorCredential records and returns local administrator credential availability.
func (r *setupBrowserRepository) HasLocalAdministratorCredential(context.Context) (bool, error) {
	r.localCredentialChecked = true
	return r.localAdminCredential, nil
}

// OIDCGroupMappings records and returns configured OIDC group mappings.
func (r *setupBrowserRepository) OIDCGroupMappings(context.Context) ([]domain.OIDCGroupMapping, error) {
	r.oidcMappingsChecked = true
	return nil, r.oidcMappingsErr
}

// LocalUserBySession records the looked-up session hash and returns the configured user.
func (r *setupBrowserRepository) LocalUserBySession(_ context.Context, tokenHash string) (domain.User, error) {
	r.sessionHash = tokenHash
	if r.sessionUser.ID == 0 {
		return domain.User{}, domain.ErrNotFound
	}

	return r.sessionUser, nil
}

// TestConfigureBrowserAuthAllowsSetupWithStaleLocalMode verifies first-run setup bypasses stale persisted local-mode validation.
func TestConfigureBrowserAuthAllowsSetupWithStaleLocalMode(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{
		settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{Mode: string(domain.AuthModeLocal)},
		},
		setupRequired: true,
	}

	configured, err := ConfigureBrowserAuth(context.Background(), BrowserConfig{}, repository)

	require.NoError(t, err)
	assert.NotNil(t, configured.Authenticator)
	assert.False(t, repository.localCredentialChecked)
}

// TestConfigureBrowserAuthAllowsSetupWithRuntimeOIDCOverride verifies first-run setup remains available with an OIDC runtime override.
func TestConfigureBrowserAuthAllowsSetupWithRuntimeOIDCOverride(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{setupRequired: true}

	configured, err := ConfigureBrowserAuth(
		context.Background(),
		BrowserConfig{ModeOverride: domain.AuthModeOIDC},
		repository,
	)

	require.NoError(t, err)
	assert.NotNil(t, configured.Authenticator)
	assert.False(t, repository.localCredentialChecked)
}

// TestBrowserLoginRedirectsSetupWithStaleLocalMode verifies login redirects to setup before stale local configuration is enforced.
func TestBrowserLoginRedirectsSetupWithStaleLocalMode(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{
		settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{Mode: string(domain.AuthModeLocal)},
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

// TestBrowserLoginRedirectsSetupWithRuntimeOIDCOverride verifies login redirects to setup before runtime OIDC configuration is enforced.
func TestBrowserLoginRedirectsSetupWithRuntimeOIDCOverride(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{setupRequired: true}
	browser := &browserAuthenticator{
		repository:   repository,
		modeOverride: domain.AuthModeOIDC,
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/login", nil)

	browser.login(response, request)

	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/setup", response.Header().Get("Location"))
}

// TestBrowserAuthenticateUsesBootstrapSessionWithRuntimeOverride verifies setup bootstrap sessions work while an external runtime override is active.
func TestBrowserAuthenticateUsesBootstrapSessionWithRuntimeOverride(t *testing.T) {
	t.Parallel()

	const token = "setup-bootstrap-token"
	repository := &setupBrowserRepository{
		setupRequired: false,
		sessionUser: domain.User{
			ID:      7,
			Role:    "admin",
			Enabled: true,
		},
	}
	browser := &browserAuthenticator{
		repository:   repository,
		modeOverride: domain.AuthModeOIDC,
		local:        NewLocal(repository, "http://localhost:8080"),
	}
	request := httptest.NewRequest(http.MethodGet, "/admin/configuration", nil)
	request.AddCookie(&http.Cookie{Name: bootstrapSessionCookie, Value: token})

	user, err := browser.Authenticate(request)

	require.NoError(t, err)
	assert.Equal(t, int64(7), user.ID)
	assert.Equal(t, bootstrapSessionHash(token), repository.sessionHash)
	assert.NotEqual(t, localSessionHash(token), repository.sessionHash)
}

// TestBrowserValidationStillRequiresLocalAdministratorAfterSetup verifies local mode requires a usable administrator credential after setup.
func TestBrowserValidationStillRequiresLocalAdministratorAfterSetup(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{setupRequired: false}
	browser := &browserAuthenticator{repository: repository}

	err := browser.validate(context.Background(), domain.AuthenticationSettings{Mode: string(domain.AuthModeLocal)})

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "auth_mode", validation.Fields[0].Field)
	assert.Equal(t, "Local authentication requires an administrator with a local password.", validation.UserMessage())
	assert.True(t, repository.localCredentialChecked)
}

// TestBrowserCurrentSettingsOverlaysRuntimeManagedFields verifies runtime-managed authentication values override persisted fields consistently.
func TestBrowserCurrentSettingsOverlaysRuntimeManagedFields(t *testing.T) {
	t.Parallel()

	t.Run("OIDC", func(t *testing.T) {
		t.Parallel()

		repository := &setupBrowserRepository{settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{
				Mode:           string(domain.AuthModeLocal),
				OIDCIssuer:     "https://stored.example.test",
				OIDCClientID:   "stored-client",
				OIDCGroupClaim: "groups",
				OIDCAdminGroup: "/admins",
			},
		}}
		browser := &browserAuthenticator{
			repository:   repository,
			modeOverride: domain.AuthModeOIDC,
			oidcConfig: OIDCConfig{
				Issuer:     "https://runtime.example.test",
				ClientID:   "runtime-client",
				GroupClaim: "roles",
				AdminGroup: "/runtime-admins",
			},
		}

		settings, err := browser.currentSettings(context.Background())

		require.NoError(t, err)
		assert.Equal(t, string(domain.AuthModeOIDC), settings.Mode)
		assert.Equal(t, "https://runtime.example.test", settings.OIDCIssuer)
		assert.Equal(t, "runtime-client", settings.OIDCClientID)
		assert.Equal(t, "roles", settings.OIDCGroupClaim)
		assert.Equal(t, "/runtime-admins", settings.OIDCAdminGroup)
	})

	t.Run("trusted proxy", func(t *testing.T) {
		t.Parallel()

		repository := &setupBrowserRepository{settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{
				Mode:                      string(domain.AuthModeLocal),
				TrustedUsernameHeaders:    []string{"Stored-User"},
				TrustedEmailHeaders:       []string{"Stored-Email"},
				TrustedDisplayNameHeaders: []string{"Stored-Name"},
				TrustedGroupHeaders:       []string{"X-Groups"},
				TrustedAdminGroup:         "admins",
			},
		}}
		browser := &browserAuthenticator{
			repository:   repository,
			modeOverride: domain.AuthModeTrustedProxy,
			trustedProxy: TrustedProxyHeaders{
				Username:    []string{"Runtime-User"},
				Email:       []string{"Runtime-Email"},
				DisplayName: []string{"Runtime-Name"},
				Groups:      []string{"Runtime-Groups"},
				AdminGroup:  "runtime-admins",
			},
		}

		repository.settings.Authentication.OIDCGroupSync = true
		repository.oidcMappingsErr = errors.New("stale OIDC mappings must not be loaded")

		settings, err := browser.currentSettings(context.Background())

		require.NoError(t, err)
		assert.False(t, repository.oidcMappingsChecked)
		assert.Equal(t, string(domain.AuthModeTrustedProxy), settings.Mode)
		assert.Equal(t, []string{"Runtime-User"}, settings.TrustedUsernameHeaders)
		assert.Equal(t, []string{"Runtime-Email"}, settings.TrustedEmailHeaders)
		assert.Equal(t, []string{"Runtime-Name"}, settings.TrustedDisplayNameHeaders)
		assert.Equal(t, []string{"Runtime-Groups"}, settings.TrustedGroupHeaders)
		assert.Equal(t, "runtime-admins", settings.TrustedAdminGroup)
	})
}
