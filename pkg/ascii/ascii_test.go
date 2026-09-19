package ascii

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAlphanumeric(t *testing.T) {
	t.Parallel()

	t.Run("lowercase a", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('a'))
	})

	t.Run("lowercase z", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('z'))
	})

	t.Run("uppercase A", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('A'))
	})

	t.Run("uppercase Z", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('Z'))
	})

	t.Run("digit zero", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('0'))
	})

	t.Run("digit nine", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('9'))
	})

	t.Run("hyphen", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsAlphanumeric('-'))
	})

	t.Run("underscore", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsAlphanumeric('_'))
	})

	t.Run("period", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsAlphanumeric('.'))
	})

	t.Run("slash", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsAlphanumeric('/'))
	})

	t.Run("space", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsAlphanumeric(' '))
	})

	t.Run("non-ASCII letter", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsAlphanumeric('\u00e9'))
	})
}
