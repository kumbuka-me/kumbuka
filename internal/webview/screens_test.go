package webview

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditorToolbarRendersResolvedDirectActionsAndSubmenus(t *testing.T) {
	views, err := New(web.Assets, testViewsLogger(), "test", "test", nil, RuntimeInfo{})
	require.NoError(t, err)
	strike := plugin.EditorInsertContribution{PluginID: "io.example.strike", ModuleID: "strike", Name: "Strikethrough", Markdown: "~~", Suffix: "~~", Mode: "wrap"}
	callouts := []plugin.EditorInsertContribution{{PluginID: "io.example.callouts", ModuleID: "note", Name: "Note", Markdown: "!!! note", Mode: "insert"}, {PluginID: "io.example.callouts", ModuleID: "danger", Name: "Danger", Markdown: "!!! danger", Mode: "insert"}}
	model := EditView{Layout: Layout{Title: "Edit", User: domain.User{ID: 1, Role: "admin"}, Preferences: domain.DefaultUserPreferences(), EditorToolbar: []plugin.ToolbarGroup{
		{ID: "text", Label: "Text formatting", Contributions: []plugin.ToolbarContribution{{ID: "io.example.strike:strike", Name: "Strikethrough", Icon: "braces-lucide", Action: &strike}}},
		{ID: "insert", Label: "Insert", Contributions: []plugin.ToolbarContribution{{ID: "io.example.callouts:callouts", Name: "Callouts", Icon: "braces-lucide", Children: callouts}}},
	}}, Page: &domain.Page{Slug: "docs/start", Title: "Start"}}
	response := httptest.NewRecorder()
	views.Render(response, "edit", model)
	require.Equal(t, 200, response.Code, response.Body.String())
	body := response.Body.String()
	assert.Equal(t, 1, strings.Count(body, `data-toolbar-contribution="io.example.callouts:callouts"`))
	assert.Contains(t, body, `data-plugin-insert-name="Note"`)
	assert.Contains(t, body, `data-plugin-insert-name="Danger"`)
	assert.Equal(t, 1, strings.Count(body, `data-toolbar-contribution="io.example.strike:strike"`))
	assert.Contains(t, body, `data-plugin-insert-suffix="~~"`)
}

func TestTypedScreensRenderProductionTemplates(t *testing.T) {
	t.Parallel()
	views, err := New(web.Assets, testViewsLogger(), "test", "test", nil, RuntimeInfo{})
	require.NoError(t, err)
	layout := Layout{Title: "Screen", User: domain.User{ID: 1, Role: "admin"}, Preferences: domain.DefaultUserPreferences()}
	cases := []struct {
		name  string
		model Screen
	}{
		{"admin_templates", AdminTemplatesView{Layout: layout}},
		{"admin_configuration", AdminConfigurationView{Layout: layout}},
		{"admin_health", AdminHealthView{Layout: layout}},
		{"admin_audit", AdminAuditView{Layout: layout}},
		{"admin_tokens", AdminTokensView{Layout: layout}},
		{"admin_exports", AdminExportsView{Layout: layout}},
		{"admin_images", AdminImagesView{Layout: layout}},
		{"admin_bin", AdminBinView{Layout: layout}},
		{"review", ReviewView{Layout: layout, Page: &domain.Page{Slug: "docs/start", Title: "Start"}}},
		{"page", PageView{Layout: layout, Page: &domain.Page{Slug: "docs/start", Title: "Start"}}},
		{"login", AuthenticationView{Layout: layout}},
		{"setup", AuthenticationView{Layout: layout}},
		{"admin_users", AdminUsersView{Layout: layout}},
		{"admin_webhooks", AdminWebhooksView{Layout: layout}},
		{"search", SearchView{Layout: layout}},
		{"admin_groups", AdminGroupsView{Layout: layout}},
		{"admin_navigation", AdminNavigationView{Layout: layout}},
		{"admin_tags", AdminTagsView{Layout: layout}},
		{"settings", SettingsView{Layout: layout}},
		{"admin_permissions", AdminPermissionsView{Layout: layout}},
		{"admin", AdminView{Layout: layout}},
		{"home", HomeView{Layout: layout}},
		{"edit", EditView{Layout: layout, Page: &domain.Page{Slug: "docs/start", Title: "Start"}}},
		{"admin_plugins", AdminPluginsView{Layout: layout}},
		{"admin_plugin_settings", AdminPluginSettingsView{Layout: layout, PluginSettings: &plugin.LoadedPlugin{}}},
		{"admin_pages", AdminPagesView{Layout: layout}},
		{"graph", GraphView{Layout: layout}},
		{"admin_branding", layout},
		{"admin_import", AdminImportView{Layout: layout}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			switch test.name {
			case "login", "setup":
				views.RenderPublic(response, test.name, test.model)
			default:
				views.Render(response, test.name, test.model)
			}
			require.Equal(t, 200, response.Code, response.Body.String())
			require.Contains(t, response.Header().Get("Content-Type"), "text/html")
		})
	}
}
