package routes

import (
	"net/http"
	"testing"
)

// TestPageReviewSuggestionRoutesDoNotConflict verifies single and bulk apply patterns can coexist in ServeMux.
func TestPageReviewSuggestionRoutesDoNotConflict(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle(pageReviewSuggestionApplyPattern, http.NotFoundHandler())
	mux.Handle(pageReviewSuggestionsApplyAllPattern, http.NotFoundHandler())
}
