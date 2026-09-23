package endpoint

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidDynamicFormRow(t *testing.T) {
	t.Parallel()

	t.Run("accepts portable identifier", func(t *testing.T) {
		t.Parallel()

		assert.True(t, validDynamicFormRow("header-42_value"))
	})

	t.Run("rejects empty identifier", func(t *testing.T) {
		t.Parallel()

		assert.False(t, validDynamicFormRow(""))
	})

	t.Run("rejects punctuation", func(t *testing.T) {
		t.Parallel()

		assert.False(t, validDynamicFormRow("header.value"))
	})

	t.Run("rejects non ASCII letters", func(t *testing.T) {
		t.Parallel()

		assert.False(t, validDynamicFormRow("héader"))
	})

	t.Run("rejects oversized identifier", func(t *testing.T) {
		t.Parallel()

		assert.False(t, validDynamicFormRow("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789___"))
	})
}
