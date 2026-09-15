package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeAuthNext(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/admin/configuration", safeAuthNext(" /admin/configuration "))
	assert.Empty(t, safeAuthNext("https://example.com"))
	assert.Empty(t, safeAuthNext("//example.com"))
}
