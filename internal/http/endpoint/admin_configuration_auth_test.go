package endpoint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// authenticationSettingsSaveStub records authentication persistence for cache-handoff tests.
type authenticationSettingsSaveStub struct {
	settingsService
	saved domain.AuthenticationSettings
	err   error
}

func (s *authenticationSettingsSaveStub) SaveAuthenticationSettings(
	_ context.Context,
	settings domain.AuthenticationSettings,
	_ int64,
) error {
	s.saved = settings
	return s.err
}

func TestSaveAdminAuthenticationAppliesRuntimeSettingsAfterPersistence(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name          string
		saveErr       error
		wantStatus    int
		wantApplyCall bool
	}{
		{name: "success", wantStatus: http.StatusSeeOther, wantApplyCall: true},
		{name: "save failure", saveErr: errors.New("save failed"), wantStatus: http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := &authenticationSettingsSaveStub{err: test.saveErr}
			applied := false
			var active domain.AuthenticationSettings
			browserAuth := auth.BrowserAuth{
				Validate: func(context.Context, domain.AuthenticationSettings) error { return nil },
				ApplySettings: func(settings domain.AuthenticationSettings) {
					applied = true
					active = settings
				},
			}
			form := url.Values{
				"auth_mode":                {string(domain.AuthModeTrustedProxy)},
				"trusted_username_headers": {"X-Forwarded-User"},
			}
			request := httptest.NewRequest(
				http.MethodPost,
				"/admin/authentication",
				strings.NewReader(form.Encode()),
			)
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request = auth.WithUser(request, domain.User{ID: 7, Role: domain.UserRoleAdmin})
			response := httptest.NewRecorder()

			SaveAdminAuthentication(service, browserAuth, testHandlerViews(t, webview.RuntimeInfo{})).ServeHTTP(response, request)

			require.Equal(t, test.wantStatus, response.Code)
			assert.Equal(t, domain.AuthModeTrustedProxy, service.saved.Mode)
			assert.Equal(t, test.wantApplyCall, applied)
			if applied {
				assert.Equal(t, service.saved, active)
			}
		})
	}
}
