package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type localAuthSettingsStub struct {
	settingsService
	settings domain.ApplicationSettings
}

func (s *localAuthSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, nil
}

type localAuthSystemStub struct {
	systemService
	setupRequired bool
	completedUser domain.User
}

func (s *localAuthSystemStub) SetupRequired(context.Context) (bool, error) {
	return s.setupRequired, nil
}

func (s *localAuthSystemStub) RecordSetupCompleted(_ context.Context, user domain.User) {
	s.completedUser = user
}

type localAuthRepositoryStub struct {
	createdUser domain.User
	sessionHash string
}

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

func (s *localAuthRepositoryStub) CreateLocalSession(
	_ context.Context,
	_ int64,
	tokenHash string,
	_ time.Time,
) error {
	s.sessionHash = tokenHash
	return nil
}

func (*localAuthRepositoryStub) DeleteLocalSession(context.Context, string) error {
	return nil
}

func (*localAuthRepositoryStub) LocalCredential(context.Context, string) (domain.User, string, error) {
	return domain.User{}, "", domain.ErrNotFound
}

func (*localAuthRepositoryStub) LocalUserBySession(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrNotFound
}

func (*localAuthRepositoryStub) SetLocalCredential(context.Context, int64, string) error {
	return nil
}

func TestSafeAuthNext(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/admin/configuration", safeAuthNext(" /admin/configuration "))
	assert.Empty(t, safeAuthNext("https://example.com"))
	assert.Empty(t, safeAuthNext("//example.com"))
}

func TestLocalLoginRedirectsSetupWithRuntimeOIDCOverride(t *testing.T) {
	t.Parallel()

	settings := &localAuthSettingsStub{settings: domain.ApplicationSettings{
		Authentication: domain.AuthenticationSettings{Mode: string(auth.AuthModeNone)},
	}}
	system := &localAuthSystemStub{setupRequired: true}
	views := testHandlerViews(t, webview.RuntimeInfo{AuthModeOverride: string(auth.AuthModeOIDC)})
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

func TestSetupAllowsRuntimeOIDCOverrideAndCreatesBootstrapSession(t *testing.T) {
	t.Parallel()

	settings := &localAuthSettingsStub{settings: domain.ApplicationSettings{
		Authentication: domain.AuthenticationSettings{Mode: string(auth.AuthModeNone)},
	}}
	system := &localAuthSystemStub{setupRequired: true}
	repository := &localAuthRepositoryStub{}
	local := auth.NewLocal(repository, "http://localhost:8080")
	views := testHandlerViews(t, webview.RuntimeInfo{AuthModeOverride: string(auth.AuthModeOIDC)})
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

func TestSetupHTMLValidationUsesUnprocessableStatus(t *testing.T) {
	t.Parallel()

	settings := &localAuthSettingsStub{settings: domain.ApplicationSettings{
		Authentication: domain.AuthenticationSettings{Mode: string(auth.AuthModeNone)},
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
