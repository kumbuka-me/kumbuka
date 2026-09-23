package ascii

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsAlphanumeric verifies ASCII letters and digits are accepted.
func TestIsAlphanumeric(t *testing.T) {
	t.Parallel()

	t.Run("lowercase letter", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('a'))
	})

	t.Run("uppercase letter", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('Z'))
	})

	t.Run("digit", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsAlphanumeric('5'))
	})

	t.Run("hyphen", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsAlphanumeric('-'))
	})

	t.Run("underscore", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsAlphanumeric('_'))
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

// TestIsLetter verifies only ASCII letters are accepted.
func TestIsLetter(t *testing.T) {
	t.Parallel()

	t.Run("lowercase a", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsLetter('a'))
	})

	t.Run("lowercase z", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsLetter('z'))
	})

	t.Run("uppercase A", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsLetter('A'))
	})

	t.Run("uppercase Z", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsLetter('Z'))
	})

	t.Run("digit", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLetter('5'))
	})

	t.Run("hyphen", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLetter('-'))
	})

	t.Run("non-ASCII letter", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsLetter('\u00e9'))
	})
}

// TestIsDigit verifies only ASCII decimal digits are accepted.
func TestIsDigit(t *testing.T) {
	t.Parallel()

	t.Run("zero", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsDigit('0'))
	})

	t.Run("nine", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsDigit('9'))
	})

	t.Run("lowercase letter", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsDigit('a'))
	})

	t.Run("uppercase letter", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsDigit('Z'))
	})

	t.Run("hyphen", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsDigit('-'))
	})

	t.Run("space", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsDigit(' '))
	})

	t.Run("non-ASCII digit", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsDigit('\u0661'))
	})
}
