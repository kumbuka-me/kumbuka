package reviewsuggestions

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApply(t *testing.T) {
	t.Parallel()

	t.Run("applies ordered suggestions", func(t *testing.T) {
		t.Parallel()
		updated, err := Apply("one\ntwo\nthree\nfour", []domain.PageReviewComment{
			{Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 2, Original: "two", Replacement: "second"},
			{Side: domain.PageReviewCommentSideNew, StartLine: 4, EndLine: 4, Original: "four", Replacement: "fourth"},
		})
		require.NoError(t, err)
		assert.Equal(t, "one\nsecond\nthree\nfourth", updated)
	})

	t.Run("rejects overlap", func(t *testing.T) {
		t.Parallel()
		_, err := Apply("one\ntwo\nthree", []domain.PageReviewComment{
			{Side: domain.PageReviewCommentSideNew, StartLine: 1, EndLine: 2, Original: "one\ntwo", Replacement: "first"},
			{Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 2, Original: "two", Replacement: "second"},
		})
		assert.ErrorIs(t, err, domain.ErrReviewSuggestionConflict)
	})

	t.Run("rejects stale source", func(t *testing.T) {
		t.Parallel()
		_, err := Apply("one\nchanged\nthree", []domain.PageReviewComment{{
			Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 2, Original: "two", Replacement: "second",
		}})
		assert.ErrorIs(t, err, domain.ErrStaleReview)
	})
}
