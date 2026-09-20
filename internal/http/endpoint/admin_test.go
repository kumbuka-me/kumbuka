package endpoint

import (
	"context"
	"log/slog"
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
	views := testHandlerViews(t, webview.RuntimeInfo{
		UserRegistrationOverrideConfigured: true,
		AllowUserRegistrationOverride:      true,
	})
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

func TestPreserveRuntimeManagedAuthenticationSettings(t *testing.T) {
	t.Parallel()

	t.Run("OIDC", func(t *testing.T) {
		t.Parallel()

		current := domain.AuthenticationSettings{
			Mode:           "local",
			OIDCIssuer:     "https://stored.example.test",
			OIDCClientID:   "stored-client",
			OIDCGroupClaim: "stored-groups",
			OIDCAdminGroup: "stored-admins",
		}
		submitted := domain.AuthenticationSettings{
			Mode:           "none",
			OIDCIssuer:     "https://changed.example.test",
			OIDCClientID:   "changed-client",
			OIDCGroupClaim: "changed-groups",
			OIDCAdminGroup: "changed-admins",
		}

		settings := preserveRuntimeManagedAuthenticationSettings(
			submitted,
			current,
			webview.RuntimeInfo{AuthModeOverride: "oidc"},
		)

		assert.Equal(t, "local", settings.Mode)
		assert.Equal(t, "https://stored.example.test", settings.OIDCIssuer)
		assert.Equal(t, "stored-client", settings.OIDCClientID)
		assert.Equal(t, "stored-groups", settings.OIDCGroupClaim)
		assert.Equal(t, "stored-admins", settings.OIDCAdminGroup)
	})

	t.Run("trusted proxy", func(t *testing.T) {
		t.Parallel()

		current := domain.AuthenticationSettings{
			Mode:                      "local",
			TrustedUsernameHeaders:    []string{"Stored-User"},
			TrustedEmailHeaders:       []string{"Stored-Email"},
			TrustedDisplayNameHeaders: []string{"Stored-Name"},
			TrustedGroupHeaders:       []string{"Stored-Groups"},
			TrustedAdminGroup:         "stored-admins",
		}
		submitted := domain.AuthenticationSettings{
			Mode:                      "none",
			TrustedUsernameHeaders:    []string{"Changed-User"},
			TrustedEmailHeaders:       []string{"Changed-Email"},
			TrustedDisplayNameHeaders: []string{"Changed-Name"},
			TrustedGroupHeaders:       []string{"Changed-Groups"},
			TrustedAdminGroup:         "changed-admins",
		}

		settings := preserveRuntimeManagedAuthenticationSettings(
			submitted,
			current,
			webview.RuntimeInfo{AuthModeOverride: "trusted-proxy"},
		)

		assert.Equal(t, "local", settings.Mode)
		assert.Equal(t, []string{"Stored-User"}, settings.TrustedUsernameHeaders)
		assert.Equal(t, []string{"Stored-Email"}, settings.TrustedEmailHeaders)
		assert.Equal(t, []string{"Stored-Name"}, settings.TrustedDisplayNameHeaders)
		assert.Equal(t, []string{"Stored-Groups"}, settings.TrustedGroupHeaders)
		assert.Equal(t, "stored-admins", settings.TrustedAdminGroup)
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

	effective := effectiveAuthenticationSettings(settings, webview.RuntimeInfo{
		AuthModeOverride:       "oidc",
		OIDCIssuerOverride:     "https://runtime.example.test",
		OIDCClientIDOverride:   "runtime-client",
		OIDCGroupClaimOverride: "roles",
		OIDCAdminGroupOverride: "runtime-admins",
	})

	assert.Equal(t, "oidc", effective.Mode)
	assert.Equal(t, "https://runtime.example.test", effective.OIDCIssuer)
	assert.Equal(t, "runtime-client", effective.OIDCClientID)
	assert.Equal(t, "roles", effective.OIDCGroupClaim)
	assert.Equal(t, "runtime-admins", effective.OIDCAdminGroup)
}

func TestAdminAuthenticationTemplates(t *testing.T) {
	views := testHandlerViews(t, webview.RuntimeInfo{})

	data := webview.Layout{Runtime: webview.RuntimeInfo{
		AuthModeOverride:       "oidc",
		OIDCIssuerOverride:     "https://runtime.example.test",
		OIDCClientIDOverride:   "runtime-client",
		OIDCGroupClaimOverride: "roles",
		OIDCAdminGroupOverride: "runtime-admins",
	}}
	data.ApplicationSettings.Authentication.Mode = "none"
	html, err := views.RenderHTML("admin_configuration", "content", webview.AdminConfigurationView{Layout: data})

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
	assert.Contains(t, string(html), "KUMBUKA__OIDC_GROUP_CLAIM")
	assert.Contains(t, string(html), "KUMBUKA__OIDC_ADMIN_GROUP")
	assert.Contains(t, string(html), "roles")
	assert.Contains(t, string(html), "runtime-admins")
	assert.Contains(t, string(html), "Save authentication settings")
	assert.NotContains(t, string(html), "Saved fallback mode")
	assert.NotContains(t, string(html), "Runtime authentication override active")
	assert.NotContains(t, string(html), "Save fallback authentication settings")
	assert.Contains(t, string(html), "PDF rendering")
	assert.Contains(t, string(html), "Test endpoint")
	assert.NotContains(t, string(html), "auth-recovery-form")

	html, err = views.RenderHTML("admin_users", "content", webview.AdminUsersView{Layout: data})

	require.NoError(t, err)
	assert.Contains(t, string(html), `<details class="admin-user-local-password" data-admin-user-local-password>`)
}

func TestEffectiveTrustedProxyAuthenticationSettings(t *testing.T) {
	t.Parallel()

	effective := effectiveAuthenticationSettings(domain.AuthenticationSettings{Mode: "local"}, webview.RuntimeInfo{
		AuthModeOverride:                  "trusted-proxy",
		TrustedUsernameHeadersOverride:    []string{"Runtime-User"},
		TrustedEmailHeadersOverride:       []string{"Runtime-Email"},
		TrustedDisplayNameHeadersOverride: []string{"Runtime-Name"},
		TrustedGroupHeadersOverride:       []string{"Runtime-Groups"},
		TrustedAdminGroupOverride:         "runtime-admins",
	})

	assert.Equal(t, "trusted-proxy", effective.Mode)
	assert.Equal(t, []string{"Runtime-User"}, effective.TrustedUsernameHeaders)
	assert.Equal(t, []string{"Runtime-Email"}, effective.TrustedEmailHeaders)
	assert.Equal(t, []string{"Runtime-Name"}, effective.TrustedDisplayNameHeaders)
	assert.Equal(t, []string{"Runtime-Groups"}, effective.TrustedGroupHeaders)
	assert.Equal(t, "runtime-admins", effective.TrustedAdminGroup)
}

func TestAdminTrustedProxyRuntimeTemplate(t *testing.T) {
	t.Parallel()

	views := testHandlerViews(t, webview.RuntimeInfo{})

	data := webview.Layout{Runtime: webview.RuntimeInfo{
		AuthModeOverride:                  "trusted-proxy",
		TrustedUsernameHeadersOverride:    []string{"Runtime-User"},
		TrustedEmailHeadersOverride:       []string{"Runtime-Email"},
		TrustedDisplayNameHeadersOverride: []string{"Runtime-Name"},
		TrustedGroupHeadersOverride:       []string{"Runtime-Groups"},
		TrustedAdminGroupOverride:         "runtime-admins",
	}}
	html, err := views.RenderHTML("admin_configuration", "content", webview.AdminConfigurationView{Layout: data})

	require.NoError(t, err)
	assert.Contains(t, string(html), "KUMBUKA__TRUSTED_GROUP_HEADERS")
	assert.Contains(t, string(html), "KUMBUKA__TRUSTED_ADMIN_GROUP")
	assert.Contains(t, string(html), "Runtime-Groups")
	assert.Contains(t, string(html), "runtime-admins")
}

func TestAdminConfigurationShowsManagedDeploymentConfiguration(t *testing.T) {
	t.Parallel()

	runtime := webview.RuntimeInfo{
		ManagedConfiguration: []webview.ManagedConfigurationGroup{
			{
				Name: "Server",
				Items: []webview.ManagedConfigurationItem{
					{Name: "Database URL", Value: "Configured", Source: "Environment · KUMBUKA__DATABASE_URL"},
					{Name: "Public URL", Value: "https://kumbuka.example.test", Source: "Flag · --public-url"},
				},
			},
			{
				Name: "Logging",
				Items: []webview.ManagedConfigurationItem{
					{Name: "Access log", Value: "Enabled", Source: "Environment · KUMBUKA__ACCESS_LOG"},
				},
			},
		},
	}
	views := testHandlerViews(t, runtime)
	data := webview.AdminConfigurationView{Layout: webview.Layout{Runtime: runtime}}
	html, err := views.RenderHTML("admin_configuration", "content", data)

	require.NoError(t, err)
	body := string(html)
	assert.Contains(t, body, "<h2>Managed</h2>")
	assert.Contains(t, body, "Deployment-owned process configuration")
	assert.Contains(t, body, "Database URL")
	assert.Contains(t, body, "Configured")
	assert.Contains(t, body, "Environment · KUMBUKA__DATABASE_URL")
	assert.Contains(t, body, "https://kumbuka.example.test")
	assert.Contains(t, body, "Flag · --public-url")
	assert.Contains(t, body, "Access log")
	assert.Contains(t, body, "Environment · KUMBUKA__ACCESS_LOG")
	assert.NotContains(t, body, "<h2>Runtime</h2>")
	assert.NotContains(t, body, "Database size")
}
