package endpoint

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseConfiguredHeaderNames(t *testing.T) {
	t.Run("preserves order and removes case-insensitive duplicates", func(t *testing.T) {
		headers := parseConfiguredHeaderNames("X-User, x-user\nX-Email")

		require.Equal(t, []string{"X-User", "X-Email"}, headers)
	})

	t.Run("drops empty values", func(t *testing.T) {
		headers := parseConfiguredHeaderNames(" , \nX-User\n")

		require.Equal(t, []string{"X-User"}, headers)
	})
}
