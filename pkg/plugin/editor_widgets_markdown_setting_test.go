package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEditorWidgetMarkdownSettings validates scalar Markdown fields and table cells.
func TestEditorWidgetMarkdownSettings(t *testing.T) {
	t.Parallel()

	attributes := map[string]EditorWidgetAttribute{
		"body":   {Name: "body", Type: EditorWidgetAttributeString},
		"titles": {Name: "titles", Type: EditorWidgetAttributeList},
		"bodies": {Name: "bodies", Type: EditorWidgetAttributeList},
		"kind":   {Name: "kind", Type: EditorWidgetAttributeEnum},
	}

	t.Run("accepts scalar Markdown", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validateEditorWidgetSetting(EditorWidgetSetting{
			Type: EditorWidgetSettingMarkdown, Label: "Content", Attribute: "body",
		}, attributes))
	})
	t.Run("rejects Markdown on enum", func(t *testing.T) {
		t.Parallel()
		require.Error(t, validateEditorWidgetSetting(EditorWidgetSetting{
			Type: EditorWidgetSettingMarkdown, Label: "Type", Attribute: "kind",
		}, attributes))
	})
	t.Run("accepts Markdown table column", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, validateEditorWidgetSetting(EditorWidgetSetting{
			Type: EditorWidgetSettingTable, Label: "Tabs",
			Attributes: []string{"titles", "bodies"},
			Columns: []EditorWidgetSettingColumn{
				{Label: "Title", Type: EditorWidgetColumnText},
				{Label: "Content", Type: EditorWidgetColumnMarkdown},
			},
		}, attributes))
	})
	t.Run("rejects Markdown column bound to a scalar", func(t *testing.T) {
		t.Parallel()
		require.Error(t, validateEditorWidgetSetting(EditorWidgetSetting{
			Type: EditorWidgetSettingTable, Label: "Tabs",
			Attributes: []string{"titles", "body"},
			Columns: []EditorWidgetSettingColumn{
				{Label: "Title", Type: EditorWidgetColumnText},
				{Label: "Content", Type: EditorWidgetColumnMarkdown},
			},
		}, attributes))
	})
}
