package templates

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateValidationBeforePersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("create rejects missing name", func(t *testing.T) {
		t.Parallel()

		_, err := NewTemplates(nil).CreatePageTemplate(ctx, PageTemplateInput{Name: " "})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "name", validation.Fields[0].Field)
	})

	t.Run("update rejects missing name", func(t *testing.T) {
		t.Parallel()

		err := NewTemplates(nil).UpdatePageTemplate(ctx, 1, PageTemplateInput{Name: " "})

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "name", validation.Fields[0].Field)
	})
}
