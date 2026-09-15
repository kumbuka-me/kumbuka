package handler

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUniqueNonEmpty(t *testing.T) {
	t.Parallel()

	t.Run("trims deduplicates and preserves order", func(t *testing.T) {
		t.Parallel()

		values := []string{" guide ", "", "api", "guide", " api ", "reference"}

		result := uniqueNonEmpty(values)

		assert.Equal(t, []string{"guide", "api", "reference"}, result)
	})

	t.Run("does not mutate input", func(t *testing.T) {
		t.Parallel()

		values := []string{" guide ", "api", "guide"}
		original := slices.Clone(values)

		_ = uniqueNonEmpty(values)

		assert.Equal(t, original, values)
	})
}
