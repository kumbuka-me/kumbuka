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

func TestEditToolbarRendersContributionsInsideResolvedHostGroups(t *testing.T) {
	t.Parallel()

	views, err := New(web.Assets, testViewsLogger(), "test", "test", nil, RuntimeInfo{})
	require.NoError(t, err)

	contribution := func(id, name, group string) plugin.ToolbarContribution {
		return plugin.ToolbarContribution{
			ID:    id,
			Name:  name,
			Icon:  "braces-lucide",
			Group: group,
			Action: &plugin.EditorInsertContribution{
				PluginID: "io.example.editor",
				Name:     name,
			},
		}
	}
	layout := Layout{
		Title:       "Edit",
		User:        domain.User{ID: 1, Role: "admin"},
		Preferences: domain.DefaultUserPreferences(),
		EditorToolbar: []plugin.ToolbarGroup{
			{ID: "text", Label: "Text formatting", Contributions: []plugin.ToolbarContribution{contribution("text-action", "Text action", "text")}},
			{ID: "blocks", Label: "Blocks", Contributions: []plugin.ToolbarContribution{contribution("blocks-action", "Blocks action", "blocks")}},
			{ID: "insert", Label: "Insert", Contributions: []plugin.ToolbarContribution{contribution("insert-action", "Insert action", "insert")}},
			{ID: "tools", Label: "Tools", Contributions: []plugin.ToolbarContribution{contribution("tools-action", "Tools action", "tools")}},
			{ID: "plugins", Label: "Plugins", Contributions: []plugin.ToolbarContribution{contribution("plugins-action", "Plugins action", "plugins")}},
		},
	}
	response := httptest.NewRecorder()

	views.Render(response, "edit", EditView{Layout: layout, Page: &domain.Page{Slug: "docs/start", Title: "Start"}})

	require.Equal(t, 200, response.Code, response.Body.String())
	body := response.Body.String()
	textGroup := strings.Index(body, `aria-label="Text formatting"`)
	textAction := strings.Index(body, `data-toolbar-contribution="text-action"`)
	blocksGroup := strings.Index(body, `aria-label="Blocks"`)
	blocksAction := strings.Index(body, `data-toolbar-contribution="blocks-action"`)
	insertGroup := strings.Index(body, `aria-label="Insert"`)
	insertAction := strings.Index(body, `data-toolbar-contribution="insert-action"`)
	toolsGroup := strings.Index(body, `aria-label="Tools"`)
	toolsAction := strings.Index(body, `data-toolbar-contribution="tools-action"`)
	pluginsGroup := strings.Index(body, `data-toolbar-group="plugins"`)
	pluginsAction := strings.Index(body, `data-toolbar-contribution="plugins-action"`)

	require.NotEqual(t, -1, textGroup)
	require.NotEqual(t, -1, textAction)
	require.NotEqual(t, -1, blocksGroup)
	require.NotEqual(t, -1, blocksAction)
	require.NotEqual(t, -1, insertGroup)
	require.NotEqual(t, -1, insertAction)
	require.NotEqual(t, -1, toolsGroup)
	require.NotEqual(t, -1, toolsAction)
	require.NotEqual(t, -1, pluginsGroup)
	require.NotEqual(t, -1, pluginsAction)

	assert.True(t, textGroup < textAction && textAction < blocksGroup)
	assert.True(t, blocksGroup < blocksAction && blocksAction < insertGroup)
	assert.True(t, insertGroup < insertAction && insertAction < toolsGroup)
	assert.True(t, toolsGroup < toolsAction && toolsAction < pluginsGroup)
	assert.True(t, pluginsGroup < pluginsAction)
}
