package postgres

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSameReviewSuggestionIDsTreatsInputsAsUniqueSets verifies atomic application rejects missing or duplicate identifiers.
func TestSameReviewSuggestionIDsTreatsInputsAsUniqueSets(t *testing.T) {
	t.Parallel()

	assert.True(t, sameReviewSuggestionIDs([]int64{3, 1, 2}, []int64{1, 2, 3}))
	assert.False(t, sameReviewSuggestionIDs([]int64{1, 1}, []int64{1, 1}))
	assert.False(t, sameReviewSuggestionIDs([]int64{1, 2}, []int64{1}))
	assert.False(t, sameReviewSuggestionIDs([]int64{0}, []int64{0}))
}

func TestValidateSelectedReviewSuggestionsDistinguishesSelectionChanges(t *testing.T) {
	t.Parallel()

	suggestions := []domain.PageReviewComment{{ID: 2}, {ID: 4}}

	t.Run("accepts exact selected set", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, validateSelectedReviewSuggestions([]int64{4, 2}, suggestions, false))
	})

	t.Run("reports missing selected suggestion", func(t *testing.T) {
		t.Parallel()

		err := validateSelectedReviewSuggestions([]int64{2, 3}, suggestions, false)

		require.ErrorIs(t, err, domain.ErrNotFound)
	})

	t.Run("reports stale apply all set", func(t *testing.T) {
		t.Parallel()

		err := validateSelectedReviewSuggestions([]int64{2}, suggestions, true)

		require.ErrorIs(t, err, domain.ErrStaleReview)
	})
}
