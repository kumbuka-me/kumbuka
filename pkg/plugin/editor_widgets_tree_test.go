package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEditorWidgetTreeSetting verifies hierarchical visual-editor settings keep identity fields hidden from users.
func TestEditorWidgetTreeSetting(t *testing.T) {
	widgets, err := parseEditorWidgetDocument([]byte(`{
  "version": 1,
  "widgets": [{
    "id": "tasks",
    "name": "Tasks",
    "inline": false,
    "syntax": {"kind": "macro", "name": "tasks"},
    "attributes": [
      {"name":"texts","type":"list","required":true,"max_bytes":512,"max_items":128,"separator":"\u001f"},
      {"name":"descriptions","type":"list","max_bytes":4096,"max_items":128,"separator":"\u001f"},
      {"name":"ids","type":"list","required":true,"max_bytes":128,"max_items":128,"separator":"\u001f","unique":true},
      {"name":"parents","type":"list","max_bytes":128,"max_items":128,"separator":"\u001f"},
      {"name":"assignees","type":"list","max_bytes":128,"max_items":128,"separator":"\u001f"},
      {"name":"dues","type":"list","max_bytes":10,"max_items":128,"separator":"\u001f"}
    ],
    "settings": [{
      "type":"tree",
      "label":"Tasks",
      "attributes":["texts","descriptions","ids","parents","assignees","dues"],
      "id_attribute":"ids",
      "parent_attribute":"parents",
      "title_attribute":"texts",
      "description_attribute":"descriptions",
      "id_prefix":"task-",
      "max_depth":16,
      "fields":[
        {"attribute":"texts","label":"Task","type":"text"},
        {"attribute":"descriptions","label":"Description","type":"textarea"},
        {"attribute":"assignees","label":"Assignee","type":"mention"},
        {"attribute":"dues","label":"Due","type":"date"}
      ]
    }],
    "preview":{"kind":"card","card":{"class":"task-list","title":"Tasks"}}
  }]
}`))

	require.NoError(t, err)
	require.Len(t, widgets, 1)
	require.Len(t, widgets[0].Settings, 1)
	setting := widgets[0].Settings[0]
	require.Equal(t, EditorWidgetSettingTree, setting.Type)
	require.Equal(t, "ids", setting.IDAttribute)
	require.Equal(t, "parents", setting.ParentAttribute)
	require.Equal(t, "texts", setting.TitleAttribute)
	require.Equal(t, 16, setting.MaxDepth)
	require.Len(t, setting.Fields, 4)
}

// TestEditorWidgetTreeSettingRejectsVisibleIdentity verifies plugins cannot expose the internal tree ID as an editable field.
func TestEditorWidgetTreeSettingRejectsVisibleIdentity(t *testing.T) {
	_, err := parseEditorWidgetDocument([]byte(`{
  "version": 1,
  "widgets": [{
    "id": "tasks",
    "name": "Tasks",
    "inline": false,
    "syntax": {"kind": "macro", "name": "tasks"},
    "attributes": [
      {"name":"texts","type":"list","required":true,"separator":"\u001f"},
      {"name":"ids","type":"list","required":true,"separator":"\u001f"},
      {"name":"parents","type":"list","separator":"\u001f"}
    ],
    "settings": [{
      "type":"tree",
      "label":"Tasks",
      "attributes":["texts","ids","parents"],
      "id_attribute":"ids",
      "parent_attribute":"parents",
      "title_attribute":"texts",
      "max_depth":16,
      "fields":[
        {"attribute":"texts","label":"Task","type":"text"},
        {"attribute":"ids","label":"ID","type":"text"}
      ]
    }],
    "preview":{"kind":"card","card":{"class":"task-list","title":"Tasks"}}
  }]
}`))

	require.ErrorContains(t, err, "invalid field")
}
