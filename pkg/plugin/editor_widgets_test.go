package plugin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/require"
)

// TestEditorWidgets verifies optional plugin visual-editor contracts are isolated from normal plugin loading failures.
func TestEditorWidgets(t *testing.T) {
	t.Run("loads enabled plugin contract", func(t *testing.T) {
		t.Parallel()

		archive := editorWidgetTestArchive(t, `{
  "version": 1,
  "widgets": [{
    "id": "status",
    "name": "Status",
    "inline": true,
    "syntax": {"kind": "macro", "name": "status"},
    "attributes": [
      {"name": "id", "type": "identifier", "required": true, "max_bytes": 128},
      {"name": "set", "type": "identifier", "max_bytes": 128},
      {"name": "options", "type": "list", "max_items": 16, "separator": ";", "fallback_separator": ",", "unique": true},
      {"name": "colors", "type": "color-list", "max_items": 16, "separator": ";", "fallback_separator": ",", "aliases": {"blue": "#2563eb"}},
      {"name": "initial", "type": "string", "max_bytes": 64},
      {"name": "style", "type": "enum", "default": "solid", "values": ["solid", "outline"]}
    ],
    "settings": [
      {"type": "text", "label": "ID", "attribute": "id"},
      {"type": "text", "label": "Reusable set", "attribute": "set", "suggestions": ["workflow", "approval"]},
      {"type": "table", "label": "Statuses", "attributes": ["options", "colors"], "columns": [{"label": "Status", "type": "text"}, {"label": "Color", "type": "color"}]},
      {"type": "text", "label": "Initial", "attribute": "initial"},
      {"type": "select", "label": "Style", "attribute": "style"}
    ],
    "constraints": [
      {"kind": "exactly-one", "attributes": ["set", "options"]},
      {"kind": "same-length", "attributes": ["options", "colors"], "optional": true},
      {"kind": "member-of", "attributes": ["initial", "options"], "optional": true}
    ],
    "preview": {"kind": "badge", "badge": {"class": "kumbuka-status", "labels_attribute": "options", "colors_attribute": "colors", "style_attribute": "style", "default_label": "Status", "default_colors": ["#64748b"]}}
  }]
}`)
		manager := editorWidgetTestManager(t, archive, true)

		widgets, problems := manager.EditorWidgets()

		require.Empty(t, problems)
		require.Len(t, widgets, 1)
		require.Equal(t, "me.kumbuka.editor-widget-fixture", widgets[0].PluginID)
		require.Equal(t, "status", widgets[0].Syntax.Name)
		require.Equal(t, ",", widgets[0].Attributes[2].FallbackSeparator)
		require.True(t, widgets[0].Attributes[2].Unique)
		require.Equal(t, []string{"workflow", "approval"}, widgets[0].Settings[1].Suggestions)
		widgets[0].Settings[1].Suggestions[0] = "changed"
		widgets[0].Attributes[3].Aliases["blue"] = "#000000"
		again, _ := manager.EditorWidgets()
		require.Equal(t, "workflow", again[0].Settings[1].Suggestions[0])
		require.Equal(t, "#2563eb", again[0].Attributes[3].Aliases["blue"])

	})

	t.Run("reports invalid contract without disabling plugin", func(t *testing.T) {
		t.Parallel()

		archive := editorWidgetTestArchive(t, `{"version":1,"widgets":[{"id":"status","unknown":true}]}`)
		manager := editorWidgetTestManager(t, archive, true)

		widgets, problems := manager.EditorWidgets()

		require.Empty(t, widgets)
		require.Len(t, problems, 1)
		require.Equal(t, "me.kumbuka.editor-widget-fixture", problems[0].PluginID)
		require.Contains(t, problems[0].Message, "invalid visual editor contract")
		require.True(t, manager.Plugins()[0].Enabled)
	})

	t.Run("serves the cached contract without reparsing package bytes", func(t *testing.T) {
		t.Parallel()

		archive := editorWidgetTestArchive(t, `{
  "version": 1,
  "widgets": [{
    "id": "status",
    "name": "Status",
    "inline": true,
    "syntax": {"kind": "macro", "name": "status"},
    "attributes": [{"name": "id", "type": "identifier", "required": true}],
    "settings": [{"type": "text", "label": "ID", "attribute": "id"}],
    "preview": {"kind": "badge", "badge": {"class": "kumbuka-status", "default_label": "Status"}}
  }]
}`)
		manager := editorWidgetTestManager(t, archive, true)
		item := manager.loaded["me.kumbuka.editor-widget-fixture"]
		item.archive = []byte("not a plugin package")
		manager.loaded["me.kumbuka.editor-widget-fixture"] = item

		widgets, problems := manager.EditorWidgets()

		require.Empty(t, problems)
		require.Len(t, widgets, 1)
		require.Equal(t, "status", widgets[0].ID)
	})

	t.Run("ignores disabled plugin contract", func(t *testing.T) {
		t.Parallel()

		archive := editorWidgetTestArchive(t, `{"version":1,"widgets":[]}`)
		manager := editorWidgetTestManager(t, archive, false)

		widgets, problems := manager.EditorWidgets()

		require.Empty(t, widgets)
		require.Empty(t, problems)
	})
}

// editorWidgetTestArchive creates one valid declarative plugin package with an optional visual-editor asset.
func editorWidgetTestArchive(t *testing.T, contract string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entries := map[string]string{
		"README.md":                 "# Editor widget fixture\n",
		"plugin.yaml":               "api_version: 1\nid: me.kumbuka.editor-widget-fixture\nname: Editor widget fixture\nversion: 1.0.0\ndefault_enabled: true\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\npermissions: []\n",
		"assets/visual-editor.json": contract,
	}
	for name, data := range entries {
		file, err := writer.Create(name)
		require.NoError(t, err)
		_, err = file.Write([]byte(data))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	_, err := pluginpackage.Read(buffer.Bytes())
	require.NoError(t, err)
	return buffer.Bytes()
}

// editorWidgetTestManager creates a manager containing one test package at the requested enabled state.
func editorWidgetTestManager(t *testing.T, archive []byte, enabled bool) *Manager {
	t.Helper()

	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)
	manager := NewManager(&Registry{}, nil)
	id := pkg.Manifest().ID
	widgets, problem := editorWidgetsFromPackage(pkg)
	manager.loaded[id] = managedPlugin{
		archive: archive,
		metadata: LoadedPlugin{
			Enabled:  enabled,
			Manifest: pkg.Manifest(),
			README:   pkg.README(),
			Digest:   pkg.Digest(),
		},
		editorWidgets:       widgets,
		editorWidgetProblem: problem,
	}
	manager.order = []string{id}
	return manager
}

func TestEditorWidgetRejectsDuplicateMacro(t *testing.T) {
	widget := EditorWidgetContribution{ID: "first", Name: "First", Syntax: EditorWidgetSyntax{Kind: "macro", Name: "status"}, Attributes: []EditorWidgetAttribute{{Name: "id", Type: "identifier"}}, Settings: []EditorWidgetSetting{{Type: "text", Label: "ID", Attribute: "id"}}, Preview: EditorWidgetPreview{Kind: "badge", Badge: &EditorWidgetBadgePreview{Class: "badge"}}}
	other := widget
	other.ID = "second"
	data, err := json.Marshal(editorWidgetDocument{Version: 1, Widgets: []EditorWidgetContribution{widget, other}})
	require.NoError(t, err)
	_, err = parseEditorWidgetDocument(data)
	require.ErrorContains(t, err, "duplicate visual editor macro")
}

func TestEditorWidgetsReportConflictingPlugins(t *testing.T) {
	manager := NewManager(&Registry{}, nil)
	for _, id := range []string{"first", "second"} {
		manager.order = append(manager.order, id)
		manager.loaded[id] = managedPlugin{metadata: LoadedPlugin{Enabled: true}, editorWidgets: []EditorWidgetContribution{{PluginID: id, Syntax: EditorWidgetSyntax{Kind: "macro", Name: "status"}}}}
	}
	widgets, problems := manager.EditorWidgets()
	require.Len(t, widgets, 1)
	require.Equal(t, "first", widgets[0].PluginID)
	require.Len(t, problems, 1)
	require.Equal(t, "second", problems[0].PluginID)
	require.Contains(t, problems[0].Message, "already handled by first")
}

func TestEditorWidgetConstraintRejectsRepeatedAttributes(t *testing.T) {
	err := validateEditorWidgetConstraint(EditorWidgetConstraint{Kind: "exactly-one", Attributes: []string{"id", "id"}}, map[string]EditorWidgetAttribute{"id": {Name: "id", Type: "identifier"}})
	require.ErrorContains(t, err, "repeats attribute")
}
