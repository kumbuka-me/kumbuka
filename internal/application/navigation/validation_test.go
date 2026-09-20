package navigation

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNavigationValidationBeforePersistence(t *testing.T) {
	t.Parallel()

	err := NewNavigation(nil, nil).SetNavigationIcon(context.Background(), "page", "not-an-icon")

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "icon", validation.Fields[0].Field)
}
