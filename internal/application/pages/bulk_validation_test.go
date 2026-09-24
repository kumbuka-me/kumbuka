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

func TestBulkValidationBeforePersistence(t *testing.T) {
	t.Parallel()

	err := NewBulk(nil, nil, nil, slog.Default()).Bulk(context.Background(), BulkPageInput{
		Action: "move",
		Slugs:  []string{"guide/child"},
		Target: "guide",
	})

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "target", validation.Fields[0].Field)
}

func TestImportRejectsInvalidAndDuplicatePathsBeforePersistence(t *testing.T) {
	t.Parallel()

	t.Run("duplicate canonical path", func(t *testing.T) {
		t.Parallel()

		bulk := NewBulk(nil, nil, nil, slog.Default())
		_, err := bulk.Import(context.Background(), []ImportedPage{
			{Slug: "Run Book", Title: "First"},
			{Slug: "run-book", Title: "Second"},
		}, "markdown", domain.User{})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, "slug", validation.Fields[0].Field)
		assert.Contains(t, validation.Fields[0].Message, "run-book")
	})

	t.Run("invalid path after valid page", func(t *testing.T) {
		t.Parallel()

		bulk := NewBulk(nil, nil, nil, slog.Default())
		_, err := bulk.Import(context.Background(), []ImportedPage{
			{Slug: "guide", Title: "Guide"},
			{Slug: "invalid//path", Title: "Invalid"},
		}, "markdown", domain.User{})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, "slug", validation.Fields[0].Field)
	})
}
