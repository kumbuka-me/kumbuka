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
