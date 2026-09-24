package webview

import (
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPartitionPageComments verifies anchored threads are removed from page-level discussion with all descendants intact.
func TestPartitionPageComments(t *testing.T) {
	t.Parallel()

	resolvedAt := time.Now()
	comments := []domain.PageComment{
		{ID: 3, ParentID: 1, Body: "inline reply"},
		{ID: 2, Body: "page root"},
		{ID: 1, Anchor: "selected text", Body: "inline root", Resolved: &resolvedAt},
		{ID: 4, ParentID: 2, Body: "page reply"},
		{ID: 5, ParentID: 3, Body: "nested inline reply"},
	}

	pageComments, inlineThreads := PartitionPageComments(comments)

	require.Len(t, pageComments, 2)
	assert.Equal(t, int64(2), pageComments[0].ID)
	assert.Equal(t, int64(4), pageComments[1].ID)

	require.Len(t, inlineThreads, 1)
	assert.Equal(t, int64(1), inlineThreads[0].RootID)
	assert.Equal(t, "selected text", inlineThreads[0].Anchor)
	assert.True(t, inlineThreads[0].Resolved)
	require.Len(t, inlineThreads[0].Comments, 3)
	assert.Equal(t, int64(1), inlineThreads[0].Comments[0].ID)
	assert.Equal(t, int64(3), inlineThreads[0].Comments[1].ID)
	assert.Equal(t, int64(5), inlineThreads[0].Comments[2].ID)
}

// TestPartitionPageCommentsKeepsOrphanRepliesVisible verifies malformed reply chains are not silently discarded.
func TestPartitionPageCommentsKeepsOrphanRepliesVisible(t *testing.T) {
	t.Parallel()

	comments := []domain.PageComment{{ID: 9, ParentID: 404, Body: "orphan"}}

	pageComments, inlineThreads := PartitionPageComments(comments)

	require.Len(t, pageComments, 1)
	assert.Equal(t, int64(9), pageComments[0].ID)
	assert.Empty(t, inlineThreads)
}

// TestReviewDiffAnchorChoosesStableSide verifies removed lines anchor to old source while visible reviewed lines anchor to new source.
func TestReviewDiffAnchorChoosesStableSide(t *testing.T) {
	t.Parallel()

	side, line := reviewDiffAnchor(revision.DiffLine{Kind: revision.DiffLineRemoved, OldLine: 8})
	assert.Equal(t, domain.PageReviewCommentSideOld, side)
	assert.Equal(t, 8, line)

	side, line = reviewDiffAnchor(revision.DiffLine{Kind: revision.DiffLineContext, OldLine: 9, NewLine: 10})
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
		{Kind: revision.DiffLineRemoved, OldLine: 4, Text: "old"},
		{Kind: revision.DiffLineAdded, NewLine: 5, Text: "new"},
	}

	lines := ReviewDiffLines(diff, comments)

	require.Len(t, lines, 2)
	require.Len(t, lines[0].Comments, 1)
	require.Len(t, lines[1].Comments, 1)
	assert.Equal(t, int64(1), lines[0].Comments[0].ID)
	assert.Equal(t, int64(2), lines[1].Comments[0].ID)
}

// TestOpenReviewSuggestionCountIgnoresAppliedSuggestions verifies only pending suggestions are counted.
func TestOpenReviewSuggestionCountIgnoresAppliedSuggestions(t *testing.T) {
	t.Parallel()

	appliedAt := time.Now()
	comments := []domain.PageReviewComment{
		{ID: 1, IsSuggestion: true},
		{ID: 2, IsSuggestion: true, AppliedAt: &appliedAt},
		{ID: 3},
	}

	assert.Equal(t, 1, OpenReviewSuggestionCount(comments))
}
