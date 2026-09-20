package pagecontent

import (
	"context"
	"testing"

	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareMaterializesStableMarkdown(t *testing.T) {
	t.Parallel()

	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")

	usage, render, err := New(renderer).Prepare(context.Background(), "# Guide\n\nStatic.")

	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Contains(t, render.HTML, `<h1 id="guide">Guide</h1>`)
	assert.NotEmpty(t, render.Fingerprint)
	require.Len(t, render.Contents, 1)
	assert.Equal(t, "guide", render.Contents[0].ID)
}

func TestPrepareLeavesDynamicMarkdownUnmaterialized(t *testing.T) {
	t.Parallel()

	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")

	usage, render, err := New(renderer).Prepare(context.Background(), "{{var:environment}}")

	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Empty(t, render.Fingerprint)
	assert.Empty(t, render.HTML)
}
