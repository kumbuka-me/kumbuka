package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// localAuthSettingsStub provides controllable local auth settings behavior for tests.
type localAuthSettingsStub struct {
	settingsService
	// settings records the tings passed to set operations.
	settings domain.ApplicationSettings
}

// ApplicationSettings returns configured settings for local-auth handler tests.
func (s *localAuthSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, nil
}

// localAuthSystemStub provides controllable local auth system behavior for tests.
type localAuthSystemStub struct {
	systemService
	// setupRequired records the up required passed to set operations.
	setupRequired bool
	// completedUser configures or records the completed user value used by the fixture.
	completedUser domain.User
}

// SetupRequired returns the configured setup state for local-auth handler tests.
func (s *localAuthSystemStub) SetupRequired(context.Context) (bool, error) {
	return s.setupRequired, nil
}

// RecordSetupCompleted records the user that completed setup.
func (s *localAuthSystemStub) RecordSetupCompleted(_ context.Context, user domain.User) {
	s.completedUser = user
}

// localAuthRepositoryStub provides controllable local auth repository behavior for tests.
type localAuthRepositoryStub struct {
	// createdUser records the user passed to create operations.
	createdUser domain.User
	// sessionHash configures or records the session hash value used by the fixture.
	sessionHash string
}

// CreateInitialLocalAdministrator creates the configured initial administrator for handler tests.
func (s *localAuthRepositoryStub) CreateInitialLocalAdministrator(
	context.Context,
	string,
	string,
	string,
	string,
) (domain.User, error) {
	if s.createdUser.ID == 0 {
		s.createdUser = domain.User{ID: 1, Username: "admin", Role: "admin", Enabled: true}
	}

	return s.createdUser, nil
}

// CreateLocalSession records the local session created by setup.
func (s *localAuthRepositoryStub) CreateLocalSession(
	_ context.Context,
	_ int64,
	tokenHash string,
	_ time.Time,
) error {
	s.sessionHash = tokenHash
	return nil
}

// DeleteLocalSession implements session deletion for local-auth handler tests.
func (*localAuthRepositoryStub) DeleteLocalSession(context.Context, string) error {
	return nil
}

// LocalCredential implements credential lookup for local-auth handler tests.
func (*localAuthRepositoryStub) LocalCredential(context.Context, string) (domain.User, string, error) {
	return domain.User{}, "", domain.ErrNotFound
}

// LocalUserBySession implements session lookup for local-auth handler tests.
func (*localAuthRepositoryStub) LocalUserBySession(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrNotFound
}

// SetLocalCredential implements credential replacement for local-auth handler tests.
func (*localAuthRepositoryStub) SetLocalCredential(context.Context, int64, string) error {
	return nil
}

// TestSafeAuthNext verifies post-authentication redirects accept only local destinations.
func TestSafeAuthNext(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/admin/configuration", safeAuthNext(" /admin/configuration "))
	assert.Empty(t, safeAuthNext("https://example.com"))
	assert.Empty(t, safeAuthNext("//example.com"))
}

// TestLocalLoginRedirectsSetupWithRuntimeOIDCOverride verifies local recovery login redirects to setup while bootstrap is incomplete.
func TestLocalLoginRedirectsSetupWithRuntimeOIDCOverride(t *testing.T) {
	t.Parallel()

	settings := &localAuthSettingsStub{settings: domain.ApplicationSettings{
		Authentication: domain.AuthenticationSettings{Mode: string(domain.AuthModeNone)},
	}}
	system := &localAuthSystemStub{setupRequired: true}
	views := testHandlerViews(t, webview.RuntimeInfo{AuthModeOverride: string(domain.AuthModeOIDC)})
	handler := LocalLogin(
		settings,
		system,
		auth.BrowserAuth{LocalLoginAllowed: func(context.Context) (bool, error) { return true, nil }},
		views,
	)
	request := httptest.NewRequest(http.MethodGet, "/auth/local", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/setup", response.Header().Get("Location"))
}

// TestSetupAllowsRuntimeOIDCOverrideAndCreatesBootstrapSession verifies setup creates a purpose-bound bootstrap session for external runtime auth.
func TestSetupAllowsRuntimeOIDCOverrideAndCreatesBootstrapSession(t *testing.T) {
	t.Parallel()

	settings := &localAuthSettingsStub{settings: domain.ApplicationSettings{
		Authentication: domain.AuthenticationSettings{Mode: string(domain.AuthModeNone)},
	}}
	system := &localAuthSystemStub{setupRequired: true}
	repository := &localAuthRepositoryStub{}
	local := auth.NewLocal(repository, "http://localhost:8080")
	views := testHandlerViews(t, webview.RuntimeInfo{AuthModeOverride: string(domain.AuthModeOIDC)})
	handler := Setup(settings, system, auth.BrowserAuth{Local: local}, views)

	form := url.Values{
		"username":         {"admin"},
		"email":            {"admin@example.com"},
		"display_name":     {"Administrator"},
		"password":         {"correct-horse-battery-staple"},
		"password_confirm": {"correct-horse-battery-staple"},
	}
	request := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/admin/configuration", response.Header().Get("Location"))
	assert.NotEmpty(t, repository.sessionHash)
	assert.Equal(t, int64(1), system.completedUser.ID)

	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "kumbuka_bootstrap_session", cookies[0].Name)
	assert.NotEmpty(t, cookies[0].Value)
	assert.True(t, cookies[0].HttpOnly)
}

// TestSetupHTMLValidationUsesUnprocessableStatus verifies invalid setup forms preserve the HTML form with a 422 response.
func TestSetupHTMLValidationUsesUnprocessableStatus(t *testing.T) {
	t.Parallel()

	settings := &localAuthSettingsStub{settings: domain.ApplicationSettings{
		Authentication: domain.AuthenticationSettings{Mode: string(domain.AuthModeNone)},
	}}
	system := &localAuthSystemStub{setupRequired: true}
	views := testHandlerViews(t, webview.RuntimeInfo{})
	handler := Setup(settings, system, auth.BrowserAuth{}, views)

	form := url.Values{
		"password":         {"correct-horse-battery-staple"},
		"password_confirm": {"correct-horse-battery-staple"},
	}
	request := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), "Username is required.")
}
