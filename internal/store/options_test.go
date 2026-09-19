package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserRegistrationOverride(t *testing.T) {
	t.Parallel()

	t.Run("unset", func(t *testing.T) {
		t.Parallel()

		database := &Store{}

		_, configured := database.userRegistrationOverride()

		assert.False(t, configured)
	})

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()

		database := &Store{}
		WithUserRegistrationOverride(true)(database)

		enabled, configured := database.userRegistrationOverride()

		require.True(t, configured)
		assert.True(t, enabled)
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		database := &Store{}
		WithUserRegistrationOverride(false)(database)

		enabled, configured := database.userRegistrationOverride()

		require.True(t, configured)
		assert.False(t, enabled)
	})
}
