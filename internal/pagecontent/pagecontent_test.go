package pagecontent

import (
	"context"
	"testing"

	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareAnalyzesStaticMarkdownWithoutRenderingOnSave(t *testing.T) {
	t.Parallel()

	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")

	usage, render, err := New(renderer).Prepare(context.Background(), "# Guide\n\nStatic.")

	require.NoError(t, err)
	require.NotNil(t, usage)
	assert.Empty(t, render.HTML)
	assert.Empty(t, render.Fingerprint)
	assert.Empty(t, render.Contents)
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
