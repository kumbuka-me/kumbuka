package users

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserValidationBeforePersistence(t *testing.T) {
	t.Parallel()

	err := NewUsers(nil, nil).UpdateUser(context.Background(), 1, "invalid", true, nil, nil)

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "role", validation.Fields[0].Field)
}
