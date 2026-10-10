package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloneEditorWidget(t *testing.T) {
	t.Run("copies nested setting choices", func(t *testing.T) {
		widget := EditorWidgetContribution{Settings: []EditorWidgetSetting{{
			Choices: map[string][]EditorWidgetChoice{"source": {{Value: "original", Label: "Original"}}},
		}}}

		clone := cloneEditorWidget(widget)
		clone.Settings[0].Choices["source"][0].Label = "Changed"

		require.Equal(t, "Original", widget.Settings[0].Choices["source"][0].Label)
	})

	t.Run("copies preview tone classes", func(t *testing.T) {
		widget := EditorWidgetContribution{Preview: EditorWidgetPreview{Badge: &EditorWidgetBadgePreview{
			ToneClasses: map[string]string{"info": "notice"},
		}}}

		clone := cloneEditorWidget(widget)
		clone.Preview.Badge.ToneClasses["info"] = "changed"

		require.Equal(t, "notice", widget.Preview.Badge.ToneClasses["info"])
	})
}
