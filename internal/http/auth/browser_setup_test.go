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

// setupBrowserRepository provides test state for setup browser repository behavior.
type setupBrowserRepository struct {
	// browserRepository is embedded to provide the default interface behavior for this fixture.
	browserRepository
	// settings records the settings returned by the fixture.
	settings domain.ApplicationSettings
	// settingsCalls counts persisted settings reads.
	settingsCalls int
	// localAdminCredential controls or records whether local admin credential is active in the test.
	localAdminCredential bool
	// localCredentialChecked controls or records whether local credential checked is active in the test.
	localCredentialChecked bool
	// oidcMappingsChecked controls or records whether OIDC mappings checked is active in the test.
	oidcMappingsChecked bool
	// oidcMappingsErr configures the error returned by the test double.
	oidcMappingsErr error
	// sessionUser configures or records the session user value used by the fixture.
	sessionUser domain.User
	// sessionHash configures or records the session hash value used by the fixture.
	sessionHash string
	// trustedProxyUser is returned by trusted-proxy identity refreshes.
	trustedProxyUser domain.User
	// trustedProxyRefreshes counts trusted-proxy identity refresh calls.
	trustedProxyRefreshes int
}

// ApplicationSettings returns configured application settings for browser-auth tests.
func (r *setupBrowserRepository) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	r.settingsCalls++
	return r.settings, nil
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

// RefreshTrustedProxyUser records a trusted-proxy steady-state authentication lookup.
func (r *setupBrowserRepository) RefreshTrustedProxyUser(
	_ context.Context,
	_, _, _ string,
	_, _ bool,
) (domain.User, error) {
	r.trustedProxyRefreshes++
	if r.trustedProxyUser.ID == 0 {
		return domain.User{}, domain.ErrNotFound
	}
	return r.trustedProxyUser, nil
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
			Authentication: domain.AuthenticationSettings{Mode: domain.AuthModeLocal},
		},
	}

	configured, err := ConfigureBrowserAuth(
		context.Background(),
		BrowserConfig{SetupRequired: func() bool { return true }},
		repository,
	)

	require.NoError(t, err)
	assert.NotNil(t, configured.Authenticator)
	assert.False(t, repository.localCredentialChecked)
}

// TestConfigureBrowserAuthAllowsSetupWithRuntimeOIDCOverride verifies first-run setup remains available with an OIDC runtime override.
func TestConfigureBrowserAuthAllowsSetupWithRuntimeOIDCOverride(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{}

	configured, err := ConfigureBrowserAuth(
		context.Background(),
		BrowserConfig{ModeOverride: domain.AuthModeOIDC, SetupRequired: func() bool { return true }},
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
			Authentication: domain.AuthenticationSettings{Mode: domain.AuthModeLocal},
		},
	}
	browser := &browserAuthenticator{repository: repository, setupRequired: func() bool { return true }}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/login", nil)

	browser.login(response, request)

	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/setup", response.Header().Get("Location"))
}

// TestBrowserLoginRedirectsSetupWithRuntimeOIDCOverride verifies login redirects to setup before runtime OIDC configuration is enforced.
func TestBrowserLoginRedirectsSetupWithRuntimeOIDCOverride(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{}
	browser := &browserAuthenticator{
		repository:    repository,
		modeOverride:  domain.AuthModeOIDC,
		setupRequired: func() bool { return true },
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

	repository := &setupBrowserRepository{}
	browser := &browserAuthenticator{repository: repository}

	err := browser.validate(context.Background(), domain.AuthenticationSettings{Mode: domain.AuthModeLocal})

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
				Mode:           domain.AuthModeLocal,
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
		assert.Equal(t, domain.AuthModeOIDC, settings.Mode)
		assert.Equal(t, "https://runtime.example.test", settings.OIDCIssuer)
		assert.Equal(t, "runtime-client", settings.OIDCClientID)
		assert.Equal(t, "roles", settings.OIDCGroupClaim)
		assert.Equal(t, "/runtime-admins", settings.OIDCAdminGroup)
	})

	t.Run("trusted proxy", func(t *testing.T) {
		t.Parallel()

		repository := &setupBrowserRepository{settings: domain.ApplicationSettings{
			Authentication: domain.AuthenticationSettings{
				Mode:                      domain.AuthModeLocal,
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
		assert.Zero(t, repository.settingsCalls)
		assert.False(t, repository.oidcMappingsChecked)
		assert.Equal(t, domain.AuthModeTrustedProxy, settings.Mode)
		assert.Equal(t, []string{"Runtime-User"}, settings.TrustedUsernameHeaders)
		assert.Equal(t, []string{"Runtime-Email"}, settings.TrustedEmailHeaders)
		assert.Equal(t, []string{"Runtime-Name"}, settings.TrustedDisplayNameHeaders)
		assert.Equal(t, []string{"Runtime-Groups"}, settings.TrustedGroupHeaders)
		assert.Equal(t, "runtime-admins", settings.TrustedAdminGroup)
	})
}

// TestRuntimeTrustedProxyAuthenticateDoesNotReadApplicationSettings verifies a fixed deployment mode stays off the settings query path.
func TestRuntimeTrustedProxyAuthenticateDoesNotReadApplicationSettings(t *testing.T) {
	t.Parallel()

	repository := &setupBrowserRepository{trustedProxyUser: domain.User{ID: 7, Username: "reader", Enabled: true}}
	configured, err := ConfigureBrowserAuth(
		context.Background(),
		BrowserConfig{
			ModeOverride: domain.AuthModeTrustedProxy,
			TrustedProxy: TrustedProxyHeaders{Username: []string{"X-User"}},
		},
		repository,
	)
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodGet, "/api/pages/example", nil)
	request.Header.Set("X-User", "reader")
	user, err := configured.Authenticator.Authenticate(request)

	require.NoError(t, err)
	assert.Equal(t, int64(7), user.ID)
	assert.Equal(t, 1, repository.trustedProxyRefreshes)
	assert.Zero(t, repository.settingsCalls)
}
