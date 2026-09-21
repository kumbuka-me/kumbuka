package server

import (
	"net/http"
	"testing"
)

// TestPageReviewSuggestionRoutesDoNotConflict verifies single and bulk apply patterns can coexist in ServeMux.
func TestPageReviewSuggestionRoutesDoNotConflict(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle("POST /reviews/{id}/suggestions/apply/{commentID}/{slug...}", http.NotFoundHandler())
	mux.Handle("POST /reviews/{id}/suggestions/apply-all/{slug...}", http.NotFoundHandler())
}

// TestPageCommentSuggestionRouteDoesNotConflict verifies inline suggestion apply remains more specific than comment creation.
func TestPageCommentSuggestionRouteDoesNotConflict(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle("POST /page-comments/{slug...}", http.NotFoundHandler())
	mux.Handle("POST /page-comments/suggestions/apply/{id}/{slug...}", http.NotFoundHandler())
}
