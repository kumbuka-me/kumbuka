package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPluginEditorCodeLanguages verifies that active plugins alone contribute code languages.
func TestPluginEditorCodeLanguages(t *testing.T) {
	t.Parallel()

	manager := NewManager(&Registry{}, nil)
	manager.order = []string{"me.example.mermaid", "me.example.runbook"}
	manager.loaded[manager.order[0]] = managedPlugin{
		metadata:            LoadedPlugin{Enabled: true},
		editorCodeLanguages: []EditorCodeLanguage{{PluginID: manager.order[0], Value: "mermaid", Label: "Mermaid"}},
	}
	manager.loaded[manager.order[1]] = managedPlugin{
		metadata:            LoadedPlugin{Enabled: false},
		editorCodeLanguages: []EditorCodeLanguage{{PluginID: manager.order[1], Value: "runbook", Label: "Runbook"}},
	}

	require.Equal(t, []EditorCodeLanguage{{PluginID: manager.order[0], Value: "mermaid", Label: "Mermaid"}}, manager.EditorCodeLanguages())

	item := manager.loaded[manager.order[1]]
	item.metadata.Enabled = true
	manager.loaded[manager.order[1]] = item
	require.Len(t, manager.EditorCodeLanguages(), 2)

	item.codeLanguageProblem = "invalid catalog"
	manager.loaded[manager.order[1]] = item
	require.Len(t, manager.EditorCodeLanguages(), 1)
}
