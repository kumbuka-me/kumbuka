package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReviewerUsernames(t *testing.T) {
	t.Parallel()

	t.Run("parses mention list", func(t *testing.T) {
		t.Parallel()

		result := reviewerUsernames("@alice @bob, @carol;dave")

		assert.Equal(t, []string{"alice", "bob", "carol", "dave"}, result)
	})

	t.Run("ignores empty separators", func(t *testing.T) {
		t.Parallel()

		result := reviewerUsernames("  , ; \n\t")

		assert.Empty(t, result)
	})
}
