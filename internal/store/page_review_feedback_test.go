package store

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestApplyLockedReviewSuggestions verifies persisted source ranges produce the expected Markdown independent of service input.
func TestApplyLockedReviewSuggestions(t *testing.T) {
	t.Parallel()

	suggestions := []domain.PageReviewComment{
		{ID: 1, Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 2, Original: "two", Replacement: "second"},
		{ID: 2, Side: domain.PageReviewCommentSideNew, StartLine: 4, EndLine: 4, Original: "four", Replacement: "fourth\nextra"},
	}

	markdown, err := applyLockedReviewSuggestions("one\ntwo\nthree\nfour", suggestions)

	require.NoError(t, err)
	assert.Equal(t, "one\nsecond\nthree\nfourth\nextra", markdown)
}

// TestApplyLockedReviewSuggestionsRejectsStaleSource verifies persisted original text must still match the locked page.
func TestApplyLockedReviewSuggestionsRejectsStaleSource(t *testing.T) {
	t.Parallel()

	suggestions := []domain.PageReviewComment{
		{ID: 1, Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 2, Original: "old", Replacement: "new"},
	}

	_, err := applyLockedReviewSuggestions("one\nchanged", suggestions)

	assert.ErrorIs(t, err, domain.ErrStaleReview)
}

// TestApplyLockedReviewSuggestionsRejectsOverlap verifies overlapping persisted suggestions cannot be applied together.
func TestApplyLockedReviewSuggestionsRejectsOverlap(t *testing.T) {
	t.Parallel()

	suggestions := []domain.PageReviewComment{
		{ID: 1, Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 3, Original: "two\nthree"},
		{ID: 2, Side: domain.PageReviewCommentSideNew, StartLine: 3, EndLine: 3, Original: "three"},
	}

	_, err := applyLockedReviewSuggestions("one\ntwo\nthree", suggestions)

	assert.ErrorIs(t, err, domain.ErrReviewSuggestionConflict)
}

// TestSameReviewSuggestionIDsTreatsInputsAsUniqueSets verifies atomic application rejects missing or duplicate identifiers.
func TestSameReviewSuggestionIDsTreatsInputsAsUniqueSets(t *testing.T) {
	t.Parallel()

	assert.True(t, sameReviewSuggestionIDs([]int64{3, 1, 2}, []int64{1, 2, 3}))
	assert.False(t, sameReviewSuggestionIDs([]int64{1, 1}, []int64{1, 1}))
	assert.False(t, sameReviewSuggestionIDs([]int64{1, 2}, []int64{1}))
	assert.False(t, sameReviewSuggestionIDs([]int64{0}, []int64{0}))
}
