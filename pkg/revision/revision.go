// Package revision compares persisted page revisions for display.
package revision

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pmezard/go-difflib/difflib"
)

// Revision contains persisted revision metadata and its derived comparison.
type Revision struct {
	// ID is the database identifier.
	ID int64 `json:"id"`
	// Number is the monotonically increasing revision number.
	Number int `json:"revision_number"`
	// Author is the display name of the editor that created the revision.
	Author string `json:"author"`
	// CreatedAt is the revision creation timestamp.
	CreatedAt time.Time `json:"created_at"`
	// Message describes the change recorded by this revision.
	Message string `json:"message"`
	// Markdown is the stored content at this revision.
	Markdown string `json:"-"`
	// PreviousMarkdown is the stored content of the preceding revision.
	PreviousMarkdown string `json:"-"`
	// AddedLines is the number of lines introduced compared with the previous revision.
	AddedLines int `json:"added_lines"`
	// RemovedLines is the number of lines removed compared with the previous revision.
	RemovedLines int `json:"removed_lines"`
	// Diff contains a unified, line-oriented patch against the previous revision.
	Diff []DiffLine `json:"diff,omitempty"`
}

// DiffLine is one display line from a unified revision patch.
type DiffLine struct {
	// Kind identifies headers, hunks, context, additions, and removals for styling.
	Kind string `json:"kind"`
	// Marker is the Git-style prefix shown before content lines.
	Marker string `json:"marker,omitempty"`
	// Text is the escaped line content rendered by the template.
	Text string `json:"text"`
	// OldLine is the one-based line number on the previous-revision side, or zero when absent.
	OldLine int `json:"old_line,omitempty"`
	// NewLine is the one-based line number on the reviewed-revision side, or zero when absent.
	NewLine int `json:"new_line,omitempty"`
}

// Analyze derives line counts and a unified patch for one persisted revision.
func Analyze(record Revision) Revision {
	record.AddedLines, record.RemovedLines = lineChanges(record.PreviousMarkdown, record.Markdown)
	record.Diff = diff(record.PreviousMarkdown, record.Markdown, record.Number)

	return record
}

// AnalyzeAll derives comparisons for every revision in records.
func AnalyzeAll(records []Revision) []Revision {
	result := make([]Revision, len(records))

	for index, record := range records {
		result[index] = Analyze(record)
	}

	return result
}

// diff returns a compact unified patch with three context lines per hunk.
func diff(previous, current string, number int) []DiffLine {
	fromFile := fmt.Sprintf("revision %d", number-1)
	if number == 1 {
		fromFile = "/dev/null"
	}

	patch, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(previous),
		B:        difflib.SplitLines(current),
		FromFile: fromFile,
		ToFile:   fmt.Sprintf("revision %d", number),
		Context:  3,
	})
	if err != nil || patch == "" {
		return nil
	}

	lines := strings.Split(strings.TrimSuffix(patch, "\n"), "\n")
	result := make([]DiffLine, 0, len(lines))
	oldLine, newLine := 0, 0

	for _, line := range lines {
		item := DiffLine{Kind: "context", Text: line}

		switch {
		case strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
			item.Kind = "header"
		case strings.HasPrefix(line, "@@"):
			item.Kind = "hunk"
			oldLine, newLine = hunkStarts(line)
		case strings.HasPrefix(line, "+"):
			item.Kind, item.Marker, item.Text = "added", "+", strings.TrimPrefix(line, "+")
			item.NewLine = newLine
			newLine++
		case strings.HasPrefix(line, "-"):
			item.Kind, item.Marker, item.Text = "removed", "-", strings.TrimPrefix(line, "-")
			item.OldLine = oldLine
			oldLine++
		case strings.HasPrefix(line, " "):
			item.Marker, item.Text = " ", strings.TrimPrefix(line, " ")
			item.OldLine, item.NewLine = oldLine, newLine
			oldLine++
			newLine++
		case strings.HasPrefix(line, "\\"):
			item.Kind = "note"
		}

		result = append(result, item)
	}

	return result
}

// hunkStarts returns the previous and current one-based line starts encoded in a unified-diff hunk header.
func hunkStarts(header string) (oldLine, newLine int) {
	body, ok := strings.CutPrefix(header, "@@ -")
	if !ok {
		return 0, 0
	}

	oldRange, body, ok := strings.Cut(body, " +")
	if !ok {
		return 0, 0
	}
	newRange, _, ok := strings.Cut(body, " @@")
	if !ok {
		return 0, 0
	}

	oldLine = rangeStart(oldRange)
	newLine = rangeStart(newRange)
	return oldLine, newLine
}

// rangeStart returns the first line number from a unified-diff range token.
func rangeStart(value string) int {
	start, _, _ := strings.Cut(value, ",")
	number, err := strconv.Atoi(start)
	if err != nil {
		return 0
	}

	return number
}

// lineChanges returns line additions and removals between two revision bodies.
func lineChanges(previous, current string) (added int, removed int) {
	before := strings.Split(previous, "\n")
	after := strings.Split(current, "\n")

	if previous == "" {
		before = nil
	}
	if current == "" {
		after = nil
	}

	// Longest common subsequence gives stable, intuitive line counts without storing a diff.
	dynamic := make([]int, len(after)+1)

	for _, left := range before {
		last := 0
		for index, right := range after {
			old := dynamic[index+1]

			if left == right {
				dynamic[index+1] = last + 1
			} else if dynamic[index] > dynamic[index+1] {
				dynamic[index+1] = dynamic[index]
			}

			last = old
		}
	}

	common := dynamic[len(after)]

	return len(after) - common, len(before) - common
}
