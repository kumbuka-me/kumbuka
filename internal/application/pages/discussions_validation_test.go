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

func TestDiscussionValidationBeforePersistence(t *testing.T) {
	t.Parallel()

	_, err := NewDiscussions(nil, nil, nil, nil, slog.Default()).AddComment(
		context.Background(),
		"page",
		0,
		"",
		"",
		" ",
		domain.User{},
	)

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "body", validation.Fields[0].Field)
}
