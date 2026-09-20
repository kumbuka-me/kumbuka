package filetype

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetectImage(t *testing.T) {
	t.Parallel()

	t.Run("jpeg", func(t *testing.T) {
		t.Parallel()
		contentType, ok := DetectImage([]byte("\xff\xd8\xff\xe0payload"))
		assert.True(t, ok)
		assert.Equal(t, ImageJPEG, contentType)
	})

	t.Run("png", func(t *testing.T) {
		t.Parallel()
		contentType, ok := DetectImage([]byte("\x89PNG\r\n\x1a\npayload"))
		assert.True(t, ok)
		assert.Equal(t, ImagePNG, contentType)
	})

	t.Run("gif87a", func(t *testing.T) {
		t.Parallel()
		contentType, ok := DetectImage([]byte("GIF87apayload"))
		assert.True(t, ok)
		assert.Equal(t, ImageGIF, contentType)
	})

	t.Run("gif89a", func(t *testing.T) {
		t.Parallel()
		contentType, ok := DetectImage([]byte("GIF89apayload"))
		assert.True(t, ok)
		assert.Equal(t, ImageGIF, contentType)
	})

	t.Run("webp", func(t *testing.T) {
		t.Parallel()
		contentType, ok := DetectImage([]byte("RIFF\x00\x00\x00\x00WEBPVP8 payload"))
		assert.True(t, ok)
		assert.Equal(t, ImageWebP, contentType)
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()
		contentType, ok := DetectImage([]byte("not an image"))
		assert.False(t, ok)
		assert.Empty(t, contentType)
	})
}
