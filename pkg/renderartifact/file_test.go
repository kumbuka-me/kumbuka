package renderartifact

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/require"
)

// TestFileCapability exercises the generic file writer used by static-site builds.
func TestFileCapability(t *testing.T) {
	t.Parallel()

	t.Run("publishes content-addressed SVG at a routed path", func(t *testing.T) {
		t.Parallel()

		output := t.TempDir()
		data := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="20" height="30"/></svg>`)
		raw, err := json.Marshal(sdk.RenderedArtifactWrite{MediaType: "image/svg+xml", Data: data})
		require.NoError(t, err)

		result, err := FileCapability(output, "/docs/")(context.Background(), raw)
		require.NoError(t, err)
		ref, ok := result.(sdk.RenderedArtifactRef)
		require.True(t, ok)
		require.True(t, strings.HasPrefix(ref.URL, "/docs/assets/rendered/"))
		require.True(t, strings.HasSuffix(ref.URL, ".svg"))

		filename := filepath.Join(output, strings.TrimPrefix(ref.URL, "/docs/"))
		stored, err := os.ReadFile(filename)
		require.NoError(t, err)
		require.Equal(t, data, stored)
	})

	t.Run("rejects executable SVG", func(t *testing.T) {
		t.Parallel()

		raw, err := json.Marshal(sdk.RenderedArtifactWrite{
			MediaType: "image/svg+xml",
			Data:      []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		})
		require.NoError(t, err)
		_, err = FileCapability(t.TempDir(), "/")(context.Background(), raw)
		require.Error(t, err)
	})
}
