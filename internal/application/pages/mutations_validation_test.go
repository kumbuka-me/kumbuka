package pages

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveValidationBeforePersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("rejects same source and destination", func(t *testing.T) {
		t.Parallel()

		err := NewMutations(nil, nil, nil, slog.Default()).Move(
			ctx,
			"/guide/",
			"guide",
			domain.MovePageOptions{},
			domain.User{},
		)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "slug", validation.Fields[0].Field)
	})

	t.Run("rejects moving tree into itself", func(t *testing.T) {
		t.Parallel()

		err := NewMutations(nil, nil, nil, slog.Default()).Move(
			ctx,
			"guide",
			"guide/child",
			domain.MovePageOptions{MoveChildren: true},
			domain.User{},
		)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "slug", validation.Fields[0].Field)
	})
}
