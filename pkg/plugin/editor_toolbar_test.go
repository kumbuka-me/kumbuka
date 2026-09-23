package plugin

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEditorToolbar(t *testing.T) {
	manifest := pluginpackage.Manifest{ID: "io.example.editor", Name: "Editor tools", Modules: []pluginpackage.Module{
		{Type: "editor-insert", ID: "strike", Name: "Strikethrough", Markdown: "~~", Group: "text", AllowedGroups: []string{"text"}, Order: 20},
		{Type: "editor-insert", ID: "note", Name: "Note", Markdown: "!!! note"},
		{Type: "editor-insert", ID: "warning", Name: "Warning", Markdown: "!!! warning"},
		{Type: "editor-menu", ID: "callouts", Name: "Callouts", Group: "insert", AllowedGroups: []string{"insert", "plugins"}, Order: 20, Children: []string{"note", "warning"}},
		{Type: "editor-insert", ID: "variable", Name: "Variable", Markdown: "{{", Group: "insert", AllowedGroups: []string{"insert", "plugins"}, Order: 20},
	}}
	manager := NewManager(&Registry{}, nil)
	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{Manifest: manifest, Enabled: true}}

	t.Run("groups submenus and breaks ordering ties by stable ID", func(t *testing.T) {
		groups := manager.ResolveEditorToolbar(nil)
		insert := toolbarGroup(t, groups, "insert")
		require.Len(t, insert.Contributions, 2)
		assert.Equal(t, "io.example.editor:callouts", insert.Contributions[0].ID)
		require.Len(t, insert.Contributions[0].Children, 2)
		assert.Equal(t, "Note", insert.Contributions[0].Children[0].Name)
		assert.Equal(t, "io.example.editor:variable", insert.Contributions[1].ID)
	})

	t.Run("applies allowed move hide and order overrides", func(t *testing.T) {
		groups := manager.ResolveEditorToolbar([]domain.EditorToolbarOverride{{ID: "io.example.editor:variable", Group: "plugins", Hidden: true, Order: -5}})
		plugins := toolbarGroup(t, groups, "plugins")
		require.Len(t, plugins.Contributions, 1)
		assert.True(t, plugins.Contributions[0].Hidden)
		assert.Equal(t, -5, plugins.Contributions[0].Order)
	})

	t.Run("ignores invalid and stale overrides", func(t *testing.T) {
		groups := manager.ResolveEditorToolbar([]domain.EditorToolbarOverride{{ID: "io.example.editor:strike", Group: "tools"}, {ID: "missing:gone", Group: "text"}})
		text := toolbarGroup(t, groups, "text")
		require.Len(t, text.Contributions, 1)
		assert.Equal(t, "text", text.Contributions[0].Group)
	})

	t.Run("omits disabled plugins", func(t *testing.T) {
		item := manager.loaded[manifest.ID]
		item.metadata.Enabled = false
		manager.loaded[manifest.ID] = item
		groups := manager.ResolveEditorToolbar(nil)
		for _, group := range groups {
			assert.Empty(t, group.Contributions)
		}
	})
}

// toolbarGroup returns one resolved group required by a test.
func toolbarGroup(t *testing.T, groups []ToolbarGroup, id string) ToolbarGroup {
	t.Helper()
	for _, group := range groups {
		if group.ID == id {
			return group
		}
	}
	require.FailNow(t, "missing toolbar group", id)
	return ToolbarGroup{}
}
