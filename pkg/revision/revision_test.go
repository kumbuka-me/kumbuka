package revision

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnalyzeUsesUnifiedFormat verifies revision analysis produces the expected unified diff structure.
func TestAnalyzeUsesUnifiedFormat(t *testing.T) {
	t.Parallel()

	record := Analyze(Revision{
		Number:           2,
		PreviousMarkdown: "# Runbook\nold command\n",
		Markdown:         "# Runbook\nnew command\n",
	})

	require.NotEmpty(t, record.Diff)
	assert.True(t, hasDiffLine(record.Diff, "header", "revision 1"))
	assert.True(t, hasDiffLine(record.Diff, "hunk", "@@"))
	assert.True(t, hasDiffLine(record.Diff, "removed", "old command"))
	assert.True(t, hasDiffLine(record.Diff, "added", "new command"))
	assert.Equal(t, 1, record.AddedLines)
	assert.Equal(t, 1, record.RemovedLines)
}

// TestAnalyzeTracksOldAndNewLineNumbers verifies diff rows expose stable source positions for review comments.
func TestAnalyzeTracksOldAndNewLineNumbers(t *testing.T) {
	t.Parallel()

	record := Analyze(Revision{
		Number:           2,
		PreviousMarkdown: "first\nold\nthird\n",
		Markdown:         "first\nnew\nthird\n",
	})

	removed := findDiffLine(t, record.Diff, "removed", "old")
	added := findDiffLine(t, record.Diff, "added", "new")
	context := findDiffLine(t, record.Diff, "context", "third")

	assert.Equal(t, 2, removed.OldLine)
	assert.Zero(t, removed.NewLine)
	assert.Zero(t, added.OldLine)
	assert.Equal(t, 2, added.NewLine)
	assert.Equal(t, 3, context.OldLine)
	assert.Equal(t, 3, context.NewLine)
}

// TestAnalyzeIsEmptyWithoutContentChanges verifies metadata-only revisions do not emit a patch.
func TestAnalyzeIsEmptyWithoutContentChanges(t *testing.T) {
	t.Parallel()

	record := Analyze(Revision{Number: 3, PreviousMarkdown: "unchanged\n", Markdown: "unchanged\n"})

	assert.Empty(t, record.Diff)
	assert.Zero(t, record.AddedLines)
	assert.Zero(t, record.RemovedLines)
}

// TestAnalyzeFirstRevisionStartsAtDevNull verifies the first revision compares against an empty source.
func TestAnalyzeFirstRevisionStartsAtDevNull(t *testing.T) {
	t.Parallel()

	record := Analyze(Revision{Number: 1, Markdown: "# First page\n"})

	require.NotEmpty(t, record.Diff)
	assert.Equal(t, "header", record.Diff[0].Kind)
	assert.Contains(t, record.Diff[0].Text, "/dev/null")
}

// TestHunkStartsParsesRanges verifies unified-diff hunk ranges are converted to one-based starts.
func TestHunkStartsParsesRanges(t *testing.T) {
	t.Parallel()

	oldLine, newLine := hunkStarts("@@ -7,3 +9,5 @@ section")

	assert.Equal(t, 7, oldLine)
	assert.Equal(t, 9, newLine)
}

// hasDiffLine reports whether a diff contains one line with the requested kind and text fragment.
func hasDiffLine(lines []DiffLine, kind, text string) bool {
	return slices.ContainsFunc(lines, func(line DiffLine) bool {
		return line.Kind == kind && strings.Contains(line.Text, text)
	})
}

// findDiffLine returns the matching diff row or fails the test when no row matches.
func findDiffLine(t *testing.T, lines []DiffLine, kind, text string) DiffLine {
	t.Helper()

	for _, line := range lines {
		if line.Kind == kind && strings.Contains(line.Text, text) {
			return line
		}
	}

	require.FailNowf(t, "missing diff line", "missing %s diff line containing %q", kind, text)
	return DiffLine{}
}
