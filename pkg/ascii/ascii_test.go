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

// TestBytePredicates verifies byte scanners can use the shared ASCII helpers without conversions.
func TestBytePredicates(t *testing.T) {
	t.Parallel()

	var letter byte = 'Q'
	var digit byte = '7'
	assert.True(t, IsAlphanumeric(letter))
	assert.True(t, IsDigit(digit))
	assert.False(t, IsAlphanumeric(byte(0xc3)))
}

// TestIsHexDigit verifies hexadecimal ASCII digits are accepted.
func TestIsHexDigit(t *testing.T) {
	t.Parallel()

	for _, character := range []byte{'0', '9', 'a', 'f', 'A', 'F'} {
		assert.True(t, IsHexDigit(character), "expected %q to be hexadecimal", character)
	}
	for _, character := range []byte{'g', 'G', '-', ' '} {
		assert.False(t, IsHexDigit(character), "expected %q to be rejected", character)
	}
}

// TestToLower verifies ASCII case folding leaves non-uppercase bytes unchanged.
func TestToLower(t *testing.T) {
	t.Parallel()

	assert.Equal(t, byte('a'), ToLower(byte('A')))
	assert.Equal(t, byte('z'), ToLower(byte('Z')))
	assert.Equal(t, byte('5'), ToLower(byte('5')))
	assert.Equal(t, rune('é'), ToLower(rune('é')))
}
