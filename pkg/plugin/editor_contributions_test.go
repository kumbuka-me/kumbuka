package plugin

import (
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/require"
)

func TestDirectEditorCompletionFields(t *testing.T) {
	t.Run("accepts supported fields", func(t *testing.T) {
		fields, supported := directEditorCompletionFields([]pluginpackage.ConfigurationField{{ID: "name", Name: "Name", Type: "text", Options: []string{"unused"}}})
		require.True(t, supported)
		require.Equal(t, []EditorCompletionField{{ID: "name", Name: "Name", Type: ConfigurationFieldText, Options: []string{"unused"}}}, fields)
	})

	t.Run("rejects unsupported fields", func(t *testing.T) {
		fields, supported := directEditorCompletionFields([]pluginpackage.ConfigurationField{{ID: "items", Type: "list"}})
		require.False(t, supported)
		require.Nil(t, fields)
	})
}
