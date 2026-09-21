package notifications

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationValidationBeforePersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("mark read rejects missing notification id", func(t *testing.T) {
		t.Parallel()

		err := NewNotifications(nil).MarkNotificationRead(ctx, 1, 0)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "notification", validation.Fields[0].Field)
	})

	t.Run("mark unread rejects missing notification id", func(t *testing.T) {
		t.Parallel()

		err := NewNotifications(nil).MarkNotificationUnread(ctx, 1, 0)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "notification", validation.Fields[0].Field)
	})

	t.Run("delete rejects missing notification id", func(t *testing.T) {
		t.Parallel()

		err := NewNotifications(nil).DeleteNotification(ctx, 1, 0)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "notification", validation.Fields[0].Field)
	})

	t.Run("open rejects missing notification id", func(t *testing.T) {
		t.Parallel()

		_, err := NewNotifications(nil).OpenNotification(ctx, 1, 0)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "notification", validation.Fields[0].Field)
	})
}
