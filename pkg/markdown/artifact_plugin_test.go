package markdown

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
)

func TestPluginAffectsArtifact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		moduleType plugin.ModuleType
		want       bool
	}{
		{name: "markdown syntax", moduleType: plugin.ModuleTypeMarkdownSyntax, want: true},
		{name: "code highlighter", moduleType: plugin.ModuleTypeCodeHighlighter, want: true},
		{name: "content style", moduleType: plugin.ModuleTypeContentStyle, want: true},
		{name: "render policy", moduleType: plugin.ModuleTypeRenderPolicy, want: true},
		{name: "renderer extension", moduleType: plugin.ModuleTypeRendererExtension, want: true},
		{name: "macro", moduleType: plugin.ModuleTypeMacro, want: true},
		{name: "content substitution", moduleType: plugin.ModuleTypeContentSubstitution, want: true},
		{name: "icon resource", moduleType: plugin.ModuleTypeIconResource, want: true},
		{name: "settings", moduleType: plugin.ModuleTypeSettings, want: false},
		{name: "page action", moduleType: plugin.ModuleTypePageAction, want: false},
		{name: "editor completion", moduleType: plugin.ModuleTypeEditorCompletion, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest := pluginpackage.Manifest{Modules: []pluginpackage.Module{{Type: string(test.moduleType)}}}
			assert.Equal(t, test.want, PluginAffectsArtifact(manifest))
		})
	}
}

func TestPluginAffectsArtifactRejectsEmptyManifest(t *testing.T) {
	t.Parallel()
	assert.False(t, PluginAffectsArtifact(pluginpackage.Manifest{}))
}
