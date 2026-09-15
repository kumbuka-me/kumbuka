package markdown

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginReplacementAnnotations(t *testing.T) {
	replacements := []plugin.Replacement{{Token: "opaque-one", Value: "staging", Annotation: "a-0123456789abcdef"}}
	resolved, ranges := resolvePluginReplacements("deploy opaque-one now", replacements)
	assert.Equal(t, "deploy staging now", resolved)
	require.Len(t, ranges, 1)
	assert.Equal(t, annotationRange{start: 7, end: 14, id: "a-0123456789abcdef"}, ranges[0])
}

func TestPluginReplacementValuesAreNotRescanned(t *testing.T) {
	replacements := []plugin.Replacement{
		{Token: "opaque-one", Value: "opaque-two"},
		{Token: "opaque-two", Value: "expanded"},
	}
	resolved, _ := resolvePluginReplacements("opaque-one", replacements)
	assert.Equal(t, "opaque-two", resolved)
}
