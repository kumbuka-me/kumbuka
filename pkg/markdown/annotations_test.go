package markdown

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"
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

func TestIsAnnotationElement(t *testing.T) {
	node := &xhtml.Node{
		Type: xhtml.ElementNode,
		Data: "span",
		Attr: []xhtml.Attribute{
			{Key: "class", Val: "plugin-annotation"},
			{Key: "data-plugin-annotation", Val: "a-0123456789abcdef"},
		},
	}

	assert.True(t, isAnnotationElement(node))
	node.Attr[1].Val = ""
	assert.False(t, isAnnotationElement(node))
}
