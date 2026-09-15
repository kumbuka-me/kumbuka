package revision

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestAnalyzeIsEmptyWithoutContentChanges(t *testing.T) {
	t.Parallel()

	record := Analyze(Revision{Number: 3, PreviousMarkdown: "unchanged\n", Markdown: "unchanged\n"})

	assert.Empty(t, record.Diff)
	assert.Zero(t, record.AddedLines)
	assert.Zero(t, record.RemovedLines)
}

func TestAnalyzeFirstRevisionStartsAtDevNull(t *testing.T) {
	t.Parallel()

	record := Analyze(Revision{Number: 1, Markdown: "# First page\n"})

	require.NotEmpty(t, record.Diff)
	assert.Equal(t, "header", record.Diff[0].Kind)
	assert.Contains(t, record.Diff[0].Text, "/dev/null")
}

func hasDiffLine(lines []DiffLine, kind, text string) bool {
	return slices.ContainsFunc(lines, func(line DiffLine) bool {
		return line.Kind == kind && strings.Contains(line.Text, text)
	})
}
