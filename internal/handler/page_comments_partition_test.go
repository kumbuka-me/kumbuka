package handler

import (
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
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

	pageComments, inlineThreads := partitionPageComments(comments)

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

	pageComments, inlineThreads := partitionPageComments(comments)

	require.Len(t, pageComments, 1)
	assert.Equal(t, int64(9), pageComments[0].ID)
	assert.Empty(t, inlineThreads)
}
