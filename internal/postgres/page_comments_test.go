package postgres

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
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
