package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReviewDiffAnchorChoosesStableSide verifies removed lines anchor to old source while visible reviewed lines anchor to new source.
func TestReviewDiffAnchorChoosesStableSide(t *testing.T) {
	t.Parallel()

	side, line := reviewDiffAnchor(revision.DiffLine{Kind: "removed", OldLine: 8})
	assert.Equal(t, domain.PageReviewCommentSideOld, side)
	assert.Equal(t, 8, line)

	side, line = reviewDiffAnchor(revision.DiffLine{Kind: "context", OldLine: 9, NewLine: 10})
	assert.Equal(t, domain.PageReviewCommentSideNew, side)
	assert.Equal(t, 10, line)
}

// TestReviewDiffLinesGroupsFeedback verifies feedback is displayed at the source line where its range begins.
func TestReviewDiffLinesGroupsFeedback(t *testing.T) {
	t.Parallel()

	comments := []domain.PageReviewComment{
		{ID: 1, Side: domain.PageReviewCommentSideOld, StartLine: 4},
		{ID: 2, Side: domain.PageReviewCommentSideNew, StartLine: 5},
	}
	diff := []revision.DiffLine{
		{Kind: "removed", OldLine: 4, Text: "old"},
		{Kind: "added", NewLine: 5, Text: "new"},
	}

	lines := reviewDiffLines(diff, comments)

	require.Len(t, lines, 2)
	require.Len(t, lines[0].Comments, 1)
	require.Len(t, lines[1].Comments, 1)
	assert.Equal(t, int64(1), lines[0].Comments[0].ID)
	assert.Equal(t, int64(2), lines[1].Comments[0].ID)
}

// TestReviewLineRangeDefaultsEnd verifies a single submitted line becomes a one-line inclusive range.
func TestReviewLineRangeDefaultsEnd(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	start, end, ok := reviewLineRange(response, "12", "")

	assert.True(t, ok)
	assert.Equal(t, 12, start)
	assert.Equal(t, 12, end)
}
