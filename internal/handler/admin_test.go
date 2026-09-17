package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/service"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type applicationSettingsStub struct {
	settingsService
	current domain.ApplicationSettings
	saved   domain.ApplicationSettings
	actorID int64
}

func (s *applicationSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.current, nil
}

func (s *applicationSettingsStub) SaveApplicationSettings(
	_ context.Context,
	settings domain.ApplicationSettings,
	actorID int64,
) error {
	s.saved = settings
	s.actorID = actorID

	return nil
}

func TestSaveAdminSettingsPreservesDeploymentManagedRegistration(t *testing.T) {
	t.Parallel()

	settings := &applicationSettingsStub{
		current: domain.ApplicationSettings{AllowUserRegistration: false},
	}
	views := &Views{
		logger: slog.Default(),
		runtime: RuntimeInfo{
			UserRegistrationOverrideConfigured: true,
			AllowUserRegistrationOverride:      true,
		},
	}
	form := url.Values{
		"allow_user_registration": {"on"},
		"content_language":        {"en"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request = auth.WithUser(request, domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	SaveAdminSettings(settings, views, slog.Default())(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/admin/configuration", response.Header().Get("Location"))
	assert.False(t, settings.saved.AllowUserRegistration)
	assert.Equal(t, "en", settings.saved.ContentLanguage)
	assert.Equal(t, int64(7), settings.actorID)
}

func TestApplicationSettingsFromForm(t *testing.T) {
	t.Parallel()

	form := url.Values{
		"allow_user_registration":    {"on"},
		"discussions_enabled":        {"on"},
		"default_typography_size":    {" compact "},
		"content_language":           {" de-CH "},
		"robots_policy":              {" disallow "},
		"external_link_label":        {" Repository ", "Status"},
		"external_link_url":          {" https://github.com/kumbuka-me/kumbuka ", "https://status.example.test"},
		"external_link_icon":         {" github-simple ", ""},
		"external_link_description":  {" v2.4.1 ", ""},
		"external_link_hover_effect": {" lift ", "none"},
		"external_link_hover_text":   {" {{label }} | {{description}} ", "Status page"},
	}
	request := httptest.NewRequest("POST", "/admin/settings", strings.NewReader(form.Encode()))

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, request.ParseForm())

	settings := applicationSettingsFromForm(request)

	assert.True(t, settings.AllowUserRegistration)
	assert.True(t, settings.DiscussionsEnabled)
	assert.Equal(t, domain.TypographySizeCompact, settings.Rendering.DefaultTypographySize)
	assert.Equal(t, "de-CH", settings.ContentLanguage)
	assert.Equal(t, domain.RobotsPolicyDisallow, settings.RobotsPolicy)
	assert.Equal(t, []domain.ExternalLink{
		{Label: "Repository", URL: "https://github.com/kumbuka-me/kumbuka", Icon: "github-simple", Description: "v2.4.1", HoverEffect: "lift", HoverText: "{{label }} | {{description}}"},
		{Label: "Status", URL: "https://status.example.test", HoverEffect: "none", HoverText: "Status page"},
	}, settings.ExternalLinks)
}

func TestRenderingLanguageValidation(t *testing.T) {
	t.Parallel()

	assert.True(t, isContentLanguage("de-CH"))
	assert.True(t, isContentLanguage("en"))
	assert.False(t, isContentLanguage("invalid"))
}

func TestAuthenticationSettingsFromForm(t *testing.T) {
	t.Parallel()

	form := url.Values{
		"auth_mode":                    {"trusted-proxy"},
		"oidc_issuer":                  {" https://identity.example.com "},
		"oidc_client_id":               {" kumbuka "},
		"oidc_group_claim":             {" groups "},
		"oidc_group_sync":              {"on"},
		"oidc_groups_authoritative":    {"on"},
		"oidc_admin_group":             {" /kumbuka-admins "},
		"oidc_group_source":            {" /admins ", "/family"},
		"oidc_group_id":                {"7", "9"},
		"trusted_username_headers":     {"X-User, X-Backup-User, x-user"},
		"trusted_email_headers":        {"X-Email"},
		"trusted_display_name_headers": {"X-Name"},
		"trusted_group_headers":        {"X-Groups, X-Backup-Groups"},
		"trusted_admin_group":          {" kumbuka-admins "},
	}
	request := httptest.NewRequest("POST", "/admin/authentication", strings.NewReader(form.Encode()))

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, request.ParseForm())

	settings := authenticationSettingsFromForm(request)

	assert.Equal(t, "trusted-proxy", settings.Mode)
	assert.Equal(t, "https://identity.example.com", settings.OIDCIssuer)
	assert.Equal(t, "kumbuka", settings.OIDCClientID)
	assert.Equal(t, "groups", settings.OIDCGroupClaim)
	assert.True(t, settings.OIDCGroupSync)
	assert.True(t, settings.OIDCGroupsAuthoritative)
	assert.Equal(t, "/kumbuka-admins", settings.OIDCAdminGroup)
	assert.Equal(t, []domain.OIDCGroupMapping{
		{OIDCGroup: "/admins", GroupID: 7},
		{OIDCGroup: "/family", GroupID: 9},
	}, settings.OIDCGroupMappings)
	assert.Equal(t, []string{"X-User", "X-Backup-User"}, settings.TrustedUsernameHeaders)
	assert.Equal(t, []string{"X-Email"}, settings.TrustedEmailHeaders)
	assert.Equal(t, []string{"X-Name"}, settings.TrustedDisplayNameHeaders)
	assert.Equal(t, []string{"X-Groups", "X-Backup-Groups"}, settings.TrustedGroupHeaders)
	assert.Equal(t, "kumbuka-admins", settings.TrustedAdminGroup)
}

func TestAuthenticationSettingsProblemsRejectsInvalidGroupMappings(t *testing.T) {
	t.Parallel()

	settings := domain.AuthenticationSettings{
		Mode:                    "oidc",
		OIDCIssuer:              "https://identity.example.com",
		OIDCClientID:            "kumbuka",
		OIDCGroupSync:           true,
		OIDCGroupsAuthoritative: true,
		OIDCGroupMappings: []domain.OIDCGroupMapping{
			{OIDCGroup: "/admins", GroupID: 1},
			{OIDCGroup: "/admins", GroupID: 2},
		},
	}
	problems := authenticationSettingsProblems(settings, RuntimeInfo{
		OIDCClientSecretConfigured:  true,
		OIDCSessionSecretConfigured: true,
	})

	require.Len(t, problems, 2)
	assert.Equal(t, "oidc_group_claim", problems[0].Field)
	assert.Equal(t, "oidc_group_mapping", problems[1].Field)
}

func TestAuthenticationSettingsProblems(t *testing.T) {
	t.Parallel()

	settings := domain.AuthenticationSettings{
		Mode:         "oidc",
		OIDCIssuer:   "https://identity.example.com",
		OIDCClientID: "kumbuka",
	}
	problems := authenticationSettingsProblems(settings, RuntimeInfo{})

	require.Len(t, problems, 2)
	assert.Equal(t, "oidc_client_secret", problems[0].Field)
	assert.Equal(t, "oidc_session_secret", problems[1].Field)
}

func TestPendingOIDCIdentityID(t *testing.T) {
	t.Parallel()

	t.Run("accepts positive identifier", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("POST", "/admin/oidc/pending/42/link", nil)

		request.SetPathValue("id", "42")

		id, err := pendingOIDCIdentityID(request)

		require.NoError(t, err)
		assert.Equal(t, int64(42), id)
	})

	t.Run("rejects invalid identifier", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("POST", "/admin/oidc/pending/nope/link", nil)

		request.SetPathValue("id", "nope")

		_, err := pendingOIDCIdentityID(request)

		require.Error(t, err)
	})
}

type pendingIdentityStatusStub struct {
	oidcIdentityService
	id, actor int64
	rejected  bool
}

func (s *pendingIdentityStatusStub) SetPendingOIDCIdentityRejected(_ context.Context, id int64, rejected bool, actor int64) error {
	s.id, s.rejected, s.actor = id, rejected, actor
	return nil
}

func TestReopenPendingOIDCIdentity(t *testing.T) {
	users := &pendingIdentityStatusStub{rejected: true}
	mux := http.NewServeMux()

	mux.Handle("POST /admin/oidc/pending/{id}/reopen", ReopenPendingOIDCIdentity(users, slog.Default()))

	request := auth.WithUser(httptest.NewRequest("POST", "/admin/oidc/pending/42/reopen", nil), domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/admin/users#pending-identities", response.Header().Get("Location"))
	assert.Equal(t, int64(42), users.id)
	assert.Equal(t, int64(7), users.actor)
	assert.False(t, users.rejected)
}

type passwordUserStub struct{ input service.UserUpdateInput }

func (s *passwordUserStub) UpdateAccount(_ context.Context, input service.UserUpdateInput) error {
	s.input = input
	return nil
}

func TestUpdateAdminUserSubmitsCompleteAccountChange(t *testing.T) {
	users := &passwordUserStub{}
	form := url.Values{"role": {"admin"}, "account_enabled": {"on"}, "local_password": {"a-long-password-123"}, "local_password_confirm": {"a-long-password-123"}}
	request := httptest.NewRequest("POST", "/admin/users/7", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("id", "7")
	request = auth.WithUser(request, domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()
	UpdateAdminUser(users, &Views{}, slog.Default())(response, request)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, int64(7), users.input.UserID)
	assert.Equal(t, form.Get("local_password"), users.input.Password)
	assert.True(t, users.input.Enabled)
	assert.Equal(t, int64(7), users.input.Actor.ID)
}

func TestPreserveRuntimeManagedAuthenticationSettings(t *testing.T) {
	t.Parallel()

	t.Run("OIDC", func(t *testing.T) {
		t.Parallel()

		current := domain.AuthenticationSettings{
			Mode:         "local",
			OIDCIssuer:   "https://stored.example.test",
			OIDCClientID: "stored-client",
		}
		submitted := domain.AuthenticationSettings{
			Mode:           "none",
			OIDCIssuer:     "https://changed.example.test",
			OIDCClientID:   "changed-client",
			OIDCGroupClaim: "groups",
		}

		settings := preserveRuntimeManagedAuthenticationSettings(
			submitted,
			current,
			RuntimeInfo{AuthModeOverride: "oidc"},
		)

		assert.Equal(t, "local", settings.Mode)
		assert.Equal(t, "https://stored.example.test", settings.OIDCIssuer)
		assert.Equal(t, "stored-client", settings.OIDCClientID)
		assert.Equal(t, "groups", settings.OIDCGroupClaim)
	})

	t.Run("trusted proxy", func(t *testing.T) {
		t.Parallel()

		current := domain.AuthenticationSettings{
			Mode:                      "local",
			TrustedUsernameHeaders:    []string{"Stored-User"},
			TrustedEmailHeaders:       []string{"Stored-Email"},
			TrustedDisplayNameHeaders: []string{"Stored-Name"},
		}
		submitted := domain.AuthenticationSettings{
			Mode:                      "none",
			TrustedUsernameHeaders:    []string{"Changed-User"},
			TrustedEmailHeaders:       []string{"Changed-Email"},
			TrustedDisplayNameHeaders: []string{"Changed-Name"},
			TrustedGroupHeaders:       []string{"X-Groups"},
		}

		settings := preserveRuntimeManagedAuthenticationSettings(
			submitted,
			current,
			RuntimeInfo{AuthModeOverride: "trusted-proxy"},
		)

		assert.Equal(t, "local", settings.Mode)
		assert.Equal(t, []string{"Stored-User"}, settings.TrustedUsernameHeaders)
		assert.Equal(t, []string{"Stored-Email"}, settings.TrustedEmailHeaders)
		assert.Equal(t, []string{"Stored-Name"}, settings.TrustedDisplayNameHeaders)
		assert.Equal(t, []string{"X-Groups"}, settings.TrustedGroupHeaders)
	})
}

func TestEffectiveAuthenticationSettings(t *testing.T) {
	t.Parallel()

	settings := domain.AuthenticationSettings{
		Mode:           "local",
		OIDCIssuer:     "https://stored.example.test",
		OIDCClientID:   "stored-client",
		OIDCGroupClaim: "groups",
	}

	effective := effectiveAuthenticationSettings(settings, RuntimeInfo{
		AuthModeOverride:     "oidc",
		OIDCIssuerOverride:   "https://runtime.example.test",
		OIDCClientIDOverride: "runtime-client",
	})

	assert.Equal(t, "oidc", effective.Mode)
	assert.Equal(t, "https://runtime.example.test", effective.OIDCIssuer)
	assert.Equal(t, "runtime-client", effective.OIDCClientID)
	assert.Equal(t, "groups", effective.OIDCGroupClaim)
}

func TestAdminAuthenticationTemplates(t *testing.T) {
	views, err := NewViews(web.Assets, slog.Default(), "test", "test", nil, RuntimeInfo{})

	require.NoError(t, err)

	data := ViewData{Runtime: RuntimeInfo{
		AuthModeOverride:     "oidc",
		OIDCIssuerOverride:   "https://runtime.example.test",
		OIDCClientIDOverride: "runtime-client",
	}}
	data.ApplicationSettings.Authentication.Mode = "none"
	html, err := renderTemplateHTML(views, "admin_configuration", "content", data)

	require.NoError(t, err)
	assert.Contains(t, string(html), "Authentication mode")
	assert.Contains(t, string(html), "OpenID Connect (OIDC)")
	assert.Contains(t, string(html), "Managed by deployment")
	assert.Contains(t, string(html), "KUMBUKA__AUTH_MODE")
	assert.Contains(t, string(html), "Remove the runtime setting and restart Kumbuka to manage it here.")
	assert.Contains(t, string(html), "https://runtime.example.test")
	assert.Contains(t, string(html), "runtime-client")
	assert.Contains(t, string(html), "KUMBUKA__OIDC_ISSUER")
	assert.Contains(t, string(html), "KUMBUKA__OIDC_CLIENT_ID")
	assert.Contains(t, string(html), "Save authentication settings")
	assert.NotContains(t, string(html), "Saved fallback mode")
	assert.NotContains(t, string(html), "Runtime authentication override active")
	assert.NotContains(t, string(html), "Save fallback authentication settings")
	assert.Contains(t, string(html), "PDF rendering")
	assert.Contains(t, string(html), "Test endpoint")
	assert.NotContains(t, string(html), "auth-recovery-form")

	html, err = renderTemplateHTML(views, "admin_users", "content", data)

	require.NoError(t, err)
	assert.Contains(t, string(html), `<details class="admin-user-local-password" data-admin-user-local-password>`)
}
