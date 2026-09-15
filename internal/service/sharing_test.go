package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageShareToken(t *testing.T) {
	t.Parallel()

	token, err := newPageShareToken()

	require.NoError(t, err)
	assert.True(t, validPageShareToken(token))
	assert.Len(t, pageShareTokenHash(token), 64)
	assert.NotEqual(t, token, pageShareTokenHash(token))
}

func TestValidPageShareToken(t *testing.T) {
	t.Parallel()

	assert.False(t, validPageShareToken(""))
	assert.False(t, validPageShareToken("not-a-share-token"))
}
