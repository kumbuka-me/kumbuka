package portable

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewManifestUsesCurrentFormat(t *testing.T) {
	t.Parallel()

	manifest := NewManifest()

	assert.Equal(t, Format, manifest.Format)
	assert.Equal(t, Version, manifest.Version)
	assert.Empty(t, manifest.Pages)
}
