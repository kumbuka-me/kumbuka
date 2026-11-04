package revision

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageSnapshotChangedFields(t *testing.T) {
	t.Parallel()
	t.Run("equal metadata", func(t *testing.T) {
		t.Parallel()
		before := PageSnapshot{Title: "Page", Properties: map[string]string{"owner": "A"}}
		after := PageSnapshot{Title: "Page", Properties: map[string]string{"owner": "A"}, Tags: []string{}}
		assert.Empty(t, ChangedFields(&before, after))
	})
	t.Run("changed fields only", func(t *testing.T) {
		t.Parallel()
		before := PageSnapshot{Title: "Before", Status: "draft", Properties: map[string]string{"secret": "old"}}
		after := PageSnapshot{Title: "After", Status: "verified", Properties: map[string]string{"secret": "new"}}
		assert.Equal(t, []string{"title", "status", "properties"}, ChangedFields(&before, after))
	})
	t.Run("initial page", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, ChangedFields(nil, PageSnapshot{Title: "Created"}))
	})
	t.Run("review timestamp", func(t *testing.T) {
		t.Parallel()
		now := time.Now()
		later := now.Add(time.Minute)
		before := PageSnapshot{ReviewedAt: &now}
		after := PageSnapshot{ReviewedAt: &later}
		assert.Equal(t, []string{"reviewed at"}, ChangedFields(&before, after))
	})
}

func TestCompareSnapshotsDoesNotDisclosePropertyValues(t *testing.T) {
	t.Parallel()
	before := PageSnapshot{Title: "Old", Properties: map[string]string{"password": "credential-before"}}
	after := PageSnapshot{Title: "New", Properties: map[string]string{"password": "credential-after"}}
	diff := CompareSnapshots(before, after)
	require.Len(t, diff, 2)
	assert.Equal(t, MetadataDifference{Name: "title", Before: "Old", After: "New"}, diff[0])
	assert.Equal(t, "properties", diff[1].Name)
	assert.False(t, slices.ContainsFunc(diff, func(item MetadataDifference) bool {
		return item.Before == "credential-before" || item.After == "credential-after"
	}))
}

func TestAnalyzeAllComparesRecordedMetadata(t *testing.T) {
	t.Parallel()
	records := []Revision{
		{Number: 2, Markdown: "same", PreviousMarkdown: "same", Snapshot: &PageSnapshot{Title: "Updated"}, MetadataChanges: []string{"title"}},
		{Number: 1, Markdown: "same", Snapshot: &PageSnapshot{Title: "Original"}},
	}
	analyzed := AnalyzeAll(records)
	require.Len(t, analyzed[0].MetadataDiff, 1)
	assert.Equal(t, "title", analyzed[0].MetadataDiff[0].Name)
	assert.Equal(t, "Original", analyzed[0].MetadataDiff[0].Before)
	assert.Equal(t, "Updated", analyzed[0].MetadataDiff[0].After)
}
