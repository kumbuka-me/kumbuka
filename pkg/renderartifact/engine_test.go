package renderartifact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEngineRender exercises the generic DOM-free runtime and its SVG validation.
func TestEngineRender(t *testing.T) {
	t.Parallel()

	t.Run("renders a promised SVG", func(t *testing.T) {
		t.Parallel()

		renderer := &Engine{}
		t.Cleanup(renderer.Close)
		script := []byte(`globalThis.kumbukaRenderArtifact = async function(source, theme) {
            return '<svg xmlns="http://www.w3.org/2000/svg"><text>' + source + '-' + theme + '</text></svg>';
        };`)

		got, err := renderer.Render(context.Background(), script, "hello", "dark")
		require.NoError(t, err)
		require.Contains(t, string(got), "hello-dark")
	})

	t.Run("rejects an executable SVG", func(t *testing.T) {
		t.Parallel()

		renderer := &Engine{}
		t.Cleanup(renderer.Close)
		script := []byte(`globalThis.kumbukaRenderArtifact = async () => '<svg xmlns="http://www.w3.org/2000/svg"><script>bad()</script></svg>';`)

		_, err := renderer.Render(context.Background(), script, "hello", "default")
		require.Error(t, err)
	})
}
