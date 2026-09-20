package preferences

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferenceValidationBeforePersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("rejects invalid sidebar width", func(t *testing.T) {
		t.Parallel()

		err := NewPreferences(nil).SetSidebarWidth(ctx, 1, -1)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "sidebar_width", validation.Fields[0].Field)
	})

	t.Run("rejects invalid navigation style", func(t *testing.T) {
		t.Parallel()

		err := NewPreferences(nil).SavePreferences(ctx, 1, domain.UserPreferences{NavigationStyle: "invalid"})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "navigation_style", validation.Fields[0].Field)
	})

	t.Run("rejects invalid navigation density", func(t *testing.T) {
		t.Parallel()

		err := NewPreferences(nil).SavePreferences(ctx, 1, domain.UserPreferences{
			NavigationStyle:   domain.NavigationStyleSidebar,
			NavigationDensity: "invalid",
		})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "navigation_density", validation.Fields[0].Field)
	})
}
