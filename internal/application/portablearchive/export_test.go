package portablearchive

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortableResourceScannerSkipsMalformedReferences(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"/media/", "/attachments/"} {
		for _, malformed := range []string{"invalid/file.png", "0/file.png", "999999999999999999999999/file.png", "12/", "12"} {
			t.Run(prefix+malformed, func(t *testing.T) {
				source := prefix + malformed + " followed by " + prefix + "42/image.png"
				reference, ok := nextPortableResourceReference(source)
				require.True(t, ok)
				assert.EqualValues(t, 42, reference.ID)
				assert.Equal(t, prefix+"42/image.png", source[reference.Start:reference.End])
			})
		}
	}
	source := "/media/invalid /attachments/7/file.txt /media/42/image.png"
	reference, ok := nextPortableResourceReference(source)
	require.True(t, ok)
	assert.Equal(t, portableAttachmentResource, reference.Kind)
	assert.EqualValues(t, 7, reference.ID)
}

func TestPortableResourceScannerStopsAtMarkdownTerminators(t *testing.T) {
	t.Parallel()

	for _, source := range []string{
		"/media/42/image.png) trailing",
		"/media/42/image.png\ttrailing",
		"/media/42/image.png\ntrailing",
		`/media/42/image.png"trailing`,
		"/media/42/image.png'trailing",
	} {
		reference, ok := nextPortableResourceReference(source)
		require.True(t, ok)
		assert.Equal(t, "/media/42/image.png", source[reference.Start:reference.End])
	}
}

func TestMediaImageIDRequiresDecimalIdentifierAndFilename(t *testing.T) {
	t.Parallel()

	id, ok := MediaImageID("/media/42/image.png")
	require.True(t, ok)
	assert.EqualValues(t, 42, id)

	for _, value := range []string{"/media//image.png", "/media/4a/image.png", "/media/42/"} {
		_, ok := MediaImageID(value)
		assert.False(t, ok, "expected %q to be rejected", value)
	}
}
