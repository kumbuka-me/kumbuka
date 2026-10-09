package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEditorWidgetBodyFormatValidation(t *testing.T) {
	t.Parallel()

	attributes := map[string]EditorWidgetAttribute{
		"kind":   {Name: "kind", Type: EditorWidgetAttributeEnum},
		"body":   {Name: "body", Type: EditorWidgetAttributeString},
		"title":  {Name: "title", Type: EditorWidgetAttributeString},
		"open":   {Name: "open", Type: EditorWidgetAttributeEnum},
		"titles": {Name: "titles", Type: EditorWidgetAttributeList},
		"bodies": {Name: "bodies", Type: EditorWidgetAttributeList},
	}
	tests := []struct {
		name      string
		preview   EditorWidgetPreview
		wantError bool
	}{
		{
			name: "callout Markdown",
			preview: EditorWidgetPreview{Kind: EditorWidgetPreviewCallout, Callout: &EditorWidgetCalloutPreview{
				Class: "callout", KindAttribute: "kind", BodyAttribute: "body", BodyFormat: EditorWidgetBodyFormatMarkdown,
			}},
		},
		{
			name: "details Markdown",
			preview: EditorWidgetPreview{Kind: EditorWidgetPreviewDetails, Details: &EditorWidgetDetailsPreview{
				Class: "markdown-details", TitleAttribute: "title", OpenAttribute: "open", BodyAttribute: "body", BodyFormat: EditorWidgetBodyFormatMarkdown,
			}},
		},
		{
			name: "tabs Markdown",
			preview: EditorWidgetPreview{Kind: EditorWidgetPreviewTabs, Tabs: &EditorWidgetTabsPreview{
				Class: "markdown-tabs", ListClass: "markdown-tab-list", TabClass: "markdown-tab", PanelsClass: "markdown-tab-panels", PanelClass: "markdown-tab-panel", TitlesAttribute: "titles", BodiesAttribute: "bodies", BodyFormat: EditorWidgetBodyFormatMarkdown,
			}},
		},
		{
			name: "legacy plain text",
			preview: EditorWidgetPreview{Kind: EditorWidgetPreviewCallout, Callout: &EditorWidgetCalloutPreview{
				Class: "callout", KindAttribute: "kind", BodyAttribute: "body",
			}},
		},
		{
			name: "reject unknown format",
			preview: EditorWidgetPreview{Kind: EditorWidgetPreviewCallout, Callout: &EditorWidgetCalloutPreview{
				Class: "callout", KindAttribute: "kind", BodyAttribute: "body", BodyFormat: "html",
			}},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEditorWidgetPreview(tt.preview, attributes)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
