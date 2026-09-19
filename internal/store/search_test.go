package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchTokensPreserveQuotedFilterValues(t *testing.T) {
	t.Parallel()

	got := searchTokens(`group:"Platform Team" tag:kubernetes postgres restore`)
	want := []string{"group:Platform Team", "tag:kubernetes", "postgres", "restore"}

	assert.Equal(t, want, got)
}
