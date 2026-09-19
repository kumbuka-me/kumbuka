package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSameReviewSuggestionIDsTreatsInputsAsUniqueSets verifies atomic application rejects missing or duplicate identifiers.
func TestSameReviewSuggestionIDsTreatsInputsAsUniqueSets(t *testing.T) {
	t.Parallel()

	assert.True(t, sameReviewSuggestionIDs([]int64{3, 1, 2}, []int64{1, 2, 3}))
	assert.False(t, sameReviewSuggestionIDs([]int64{1, 1}, []int64{1, 1}))
	assert.False(t, sameReviewSuggestionIDs([]int64{1, 2}, []int64{1}))
	assert.False(t, sameReviewSuggestionIDs([]int64{0}, []int64{0}))
}
