package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEditorWidgetMentionAndDateSettings verifies generic person and calendar controls target scalar attributes.
func TestEditorWidgetMentionAndDateSettings(t *testing.T) {
	widgets, err := parseEditorWidgetDocument([]byte(`{
  "version": 1,
  "widgets": [{
    "id": "assignment",
    "name": "Assignment",
    "inline": false,
    "syntax": {"kind": "macro", "name": "assignment"},
    "attributes": [
      {"name": "assignee", "type": "string", "max_bytes": 128},
      {"name": "due", "type": "string", "max_bytes": 10}
    ],
    "settings": [
      {"type": "mention", "label": "Assignee", "attribute": "assignee"},
      {"type": "date", "label": "Due", "attribute": "due"}
    ],
    "preview": {"kind": "card", "card": {"class": "assignment", "title": "Assignment"}}
  }]
}`))

	require.NoError(t, err)
	require.Len(t, widgets, 1)
	require.Equal(t, EditorWidgetSettingMention, widgets[0].Settings[0].Type)
	require.Equal(t, EditorWidgetSettingDate, widgets[0].Settings[1].Type)
}
