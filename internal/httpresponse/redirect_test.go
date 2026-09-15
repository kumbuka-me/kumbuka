package httpresponse

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocalPath(t *testing.T) {
	t.Parallel()

	t.Run("root path", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsLocalPath("/"))
	})

	t.Run("path with query and fragment", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsLocalPath("/pages/start?search=hello#section"))
	})

	t.Run("escaped path", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsLocalPath("/pages/a%20b"))
	})

	t.Run("empty path", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLocalPath(""))
	})

	t.Run("absolute URL", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLocalPath("https://evil.example"))
	})

	t.Run("scheme-relative URL", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLocalPath("//evil.example"))
	})

	t.Run("backslash", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLocalPath("/\\evil.example"))
	})

	t.Run("tab", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLocalPath("/\t/evil.example"))
	})

	t.Run("newline", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLocalPath("/\n/evil.example"))
	})

	t.Run("carriage return", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLocalPath("/\r/evil.example"))
	})

	t.Run("null byte", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLocalPath("/\u0000evil"))
	})
}
