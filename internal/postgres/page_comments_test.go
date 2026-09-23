package postgres

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSuggestionMatchesSource verifies inline suggestion offsets must identify the exact original Markdown.
func TestSuggestionMatchesSource(t *testing.T) {
	t.Parallel()

	markdown := "before selected after"
	suggestion := domain.PageCommentSuggestion{StartByte: 7, EndByte: 15, Original: "selected"}

	assert.True(t, suggestionMatchesSource(markdown, suggestion))

	suggestion.Original = "modified"
	assert.False(t, suggestionMatchesSource(markdown, suggestion))

	suggestion = domain.PageCommentSuggestion{StartByte: -1, EndByte: 15, Original: "selected"}
	assert.False(t, suggestionMatchesSource(markdown, suggestion))

	suggestion = domain.PageCommentSuggestion{StartByte: 7, EndByte: len(markdown) + 1, Original: "selected"}
	assert.False(t, suggestionMatchesSource(markdown, suggestion))
}

func TestValidateCommentSuggestionRequiresExactCurrentResult(t *testing.T) {
	t.Parallel()

	suggestion := domain.PageCommentSuggestion{
		RevisionNumber: 4,
		StartByte:      7,
		EndByte:        15,
		Original:       "selected",
		Replacement:    "replacement",
	}

	t.Run("accepts exact result", func(t *testing.T) {
		t.Parallel()

		err := validateCommentSuggestion(4, "prefix selected suffix", "prefix replacement suffix", suggestion)

		require.NoError(t, err)
	})

	t.Run("rejects caller result mismatch", func(t *testing.T) {
		t.Parallel()

		err := validateCommentSuggestion(4, "prefix selected suffix", "prefix changed suffix", suggestion)

		require.ErrorIs(t, err, domain.ErrStaleSuggestion)
	})

	t.Run("rejects stale revision", func(t *testing.T) {
		t.Parallel()

		err := validateCommentSuggestion(5, "prefix selected suffix", "prefix replacement suffix", suggestion)

		require.ErrorIs(t, err, domain.ErrStaleSuggestion)
	})
}
