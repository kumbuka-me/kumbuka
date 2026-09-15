package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPermalinkPageID(t *testing.T) {
	t.Parallel()

	t.Run("accepts positive page identifier", func(t *testing.T) {
		t.Parallel()

		id, err := permalinkPageID("42")

		require.NoError(t, err)
		assert.Equal(t, int64(42), id)
	})

	t.Run("rejects invalid page identifier", func(t *testing.T) {
		t.Parallel()

		_, err := permalinkPageID("page")

		require.Error(t, err)
	})

	t.Run("rejects non-positive page identifier", func(t *testing.T) {
		t.Parallel()

		_, err := permalinkPageID("0")

		require.Error(t, err)
	})
}
