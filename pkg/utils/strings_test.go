package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitCommaSeparatedValues(t *testing.T) {
	t.Run("trims and drops empty values", func(t *testing.T) {
		require.Equal(t, []string{"one", "two", "three"}, SplitCommaSeparatedValues(" one, two, , three "))
	})

	t.Run("returns an empty list for blank input", func(t *testing.T) {
		require.Empty(t, SplitCommaSeparatedValues("  "))
	})
}
