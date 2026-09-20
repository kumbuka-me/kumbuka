package search

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSavedSearchValidationBeforePersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("rejects missing name", func(t *testing.T) {
		t.Parallel()

		err := NewKnowledge(nil, nil).SaveSavedSearch(ctx, 1, 0, " ", "query", false)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "name", validation.Fields[0].Field)
	})

	t.Run("rejects missing query", func(t *testing.T) {
		t.Parallel()

		err := NewKnowledge(nil, nil).SaveSavedSearch(ctx, 1, 0, "name", " ", false)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "query", validation.Fields[0].Field)
	})
}
