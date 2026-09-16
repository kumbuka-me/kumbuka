package markdown

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMermaidUsesRuntimeRegistryAndCentralSanitizer(t *testing.T) {
	ctx := context.Background()
	r := isolatedTestRenderer(t, "mermaid")
	source := "```mermaid\ngraph LR; A --> B\n```"
	rendered, err := r.Render(source)
	require.NoError(t, err)
	assert.Contains(t, rendered, `data-kumbuka-plugin="me.kumbuka.mermaid"`)
	assert.Contains(t, rendered, `data-kumbuka-module="diagrams"`)
	assert.Contains(t, rendered, "A --&gt; B")
	assert.NotContains(t, rendered, "<iframe")
	assert.NotContains(t, rendered, "<script")
	require.NoError(t, r.PluginManager().Disable(ctx, "me.kumbuka.mermaid"))
	rendered, err = r.Render(source)
	require.NoError(t, err)
	assert.NotContains(t, rendered, "data-kumbuka-plugin")
	require.NoError(t, r.PluginManager().Enable(ctx, "me.kumbuka.mermaid"))
	rendered, err = r.Render(source)
	require.NoError(t, err)
	assert.Contains(t, rendered, "data-kumbuka-plugin")
	rendered, err = r.Render("````\n" + source + "\n````")
	require.NoError(t, err)
	assert.NotContains(t, rendered, "data-kumbuka-plugin")
}
