package plugin

import (
	"archive/zip"
	"bytes"
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
	})

	t.Run("accepts every supported source and preview shape", func(t *testing.T) {
		t.Parallel()

		widgets, err := parseEditorWidgetDocument([]byte(`{
  "version": 1,
  "widgets": [
    {
      "id": "include",
      "name": "Include",
      "inline": true,
      "syntax": {"kind": "substitution", "name": "include"},
      "attributes": [{"name": "target", "type": "string", "required": true}],
      "settings": [{"type": "text", "label": "Target", "attribute": "target"}],
      "preview": {"kind": "reference", "reference": {"class": "visual-include-reference", "prefix": "Include", "value_attribute": "target"}}
    },
    {
      "id": "external-file",
      "name": "External file",
      "inline": false,
      "syntax": {"kind": "macro", "name": "external-file"},
      "attributes": [
        {"name": "path", "type": "string", "required": true},
        {"name": "note", "type": "list", "separator": "\u001f", "repeat": true, "max_bytes": 2048}
      ],
      "settings": [
        {"type": "text", "label": "Path", "attribute": "path"},
        {"type": "table", "label": "Notes", "attributes": ["note"], "columns": [{"label": "Note", "type": "textarea"}]}
      ],
      "preview": {"kind": "card", "card": {"class": "external-file", "title": "External file", "subtitle_attribute": "path"}}
    },
    {
      "id": "callout",
      "name": "Callout",
      "inline": false,
      "syntax": {"kind": "callout"},
      "attributes": [
        {"name": "kind", "type": "enum", "required": true, "values": ["note", "warning"]},
        {"name": "body", "type": "string", "required": true, "max_bytes": 16384}
      ],
      "settings": [
        {"type": "select", "label": "Type", "attribute": "kind"},
        {"type": "textarea", "label": "Body", "attribute": "body"}
      ],
      "preview": {"kind": "callout", "callout": {"class": "callout", "body_class": "callout-body", "kind_attribute": "kind", "body_attribute": "body"}}
    },
    {
      "id": "details",
      "name": "Details",
      "inline": false,
      "syntax": {"kind": "details"},
      "attributes": [
        {"name": "title", "type": "string", "required": true},
        {"name": "open", "type": "enum", "values": ["false", "true"]},
        {"name": "body", "type": "string", "required": true}
      ],
      "settings": [
        {"type": "text", "label": "Title", "attribute": "title"},
        {"type": "select", "label": "Open", "attribute": "open"},
        {"type": "textarea", "label": "Body", "attribute": "body"}
      ],
      "preview": {"kind": "details", "details": {"class": "markdown-details", "body_class": "markdown-details-body", "title_attribute": "title", "open_attribute": "open", "body_attribute": "body"}}
    },
    {
      "id": "tabs",
      "name": "Tabs",
      "inline": false,
      "syntax": {"kind": "tabs"},
      "attributes": [
        {"name": "titles", "type": "list", "required": true, "separator": "\u001f"},
        {"name": "bodies", "type": "list", "required": true, "separator": "\u001f", "max_bytes": 32768}
      ],
      "settings": [{"type": "table", "label": "Tabs", "attributes": ["titles", "bodies"], "columns": [{"label": "Title", "type": "text"}, {"label": "Body", "type": "textarea"}]}],
      "constraints": [{"kind": "same-length", "attributes": ["titles", "bodies"]}],
      "preview": {"kind": "tabs", "tabs": {"class": "markdown-tabs", "list_class": "markdown-tab-list", "tab_class": "markdown-tab", "panels_class": "markdown-tab-panels", "panel_class": "markdown-tab-panel", "titles_attribute": "titles", "bodies_attribute": "bodies"}}
    }
  ]
}`))

		require.NoError(t, err)
		require.Len(t, widgets, 5)
		require.True(t, widgets[1].Attributes[1].Repeat)
		require.Equal(t, EditorWidgetSettingTextarea, widgets[2].Settings[1].Type)
		require.Equal(t, EditorWidgetPreviewTabs, widgets[4].Preview.Kind)
	})

	t.Run("accepts resource setting with owned completion module", func(t *testing.T) {
		t.Parallel()

		archive := editorWidgetTestArchive(t, `{
  "version": 1,
  "widgets": [{
    "id": "variable",
    "name": "Variable",
    "inline": true,
    "syntax": {"kind": "substitution", "name": "var"},
    "attributes": [{"name": "name", "type": "identifier", "required": true}],
    "settings": [{"type": "resource", "label": "Variable", "attribute": "name", "completion_module_id": "completion"}],
    "preview": {"kind": "reference", "reference": {"class": "variable", "prefix": "Variable", "value_attribute": "name"}}
  }]
}`)
		manager := editorWidgetTestManager(t, archive, true)

		widgets, problems := manager.EditorWidgets()

		require.Empty(t, problems)
		require.Len(t, widgets, 1)
		require.Equal(t, "completion", widgets[0].Settings[0].CompletionModuleID)
	})

	t.Run("rejects resource setting with unknown completion module", func(t *testing.T) {
		t.Parallel()

		archive := editorWidgetTestArchive(t, `{
  "version": 1,
  "widgets": [{
    "id": "variable",
    "name": "Variable",
    "inline": true,
    "syntax": {"kind": "substitution", "name": "var"},
    "attributes": [{"name": "name", "type": "identifier", "required": true}],
    "settings": [{"type": "resource", "label": "Variable", "attribute": "name", "completion_module_id": "missing"}],
    "preview": {"kind": "reference", "reference": {"class": "variable", "prefix": "Variable", "value_attribute": "name"}}
  }]
}`)
		manager := editorWidgetTestManager(t, archive, true)

		widgets, problems := manager.EditorWidgets()

		require.Empty(t, widgets)
		require.Len(t, problems, 1)
		require.Contains(t, problems[0].Message, `unknown editor-completion module "missing"`)
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
		"plugin.yaml":               "api_version: 1\nid: me.kumbuka.editor-widget-fixture\nname: Editor widget fixture\nversion: 1.0.0\ndefault_enabled: true\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\n  - type: admin-resource\n    id: variables\n    name: Variables\n    fields:\n      - id: name\n        name: Name\n        type: text\n        required: true\n        key: true\n  - type: editor-completion\n    id: completion\n    resource: variables\n    trigger: '{{'\n    replacement: '{{var:${name}}}'\n    label_field: name\npermissions: []\n",
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
