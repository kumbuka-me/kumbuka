package groups

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupValidationBeforePersistence(t *testing.T) {
	t.Parallel()

	_, err := NewGroups(nil).CreateGroup(context.Background(), " ")

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "name", validation.Fields[0].Field)
}
