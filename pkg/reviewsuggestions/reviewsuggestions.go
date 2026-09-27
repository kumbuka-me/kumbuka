// Package reviewsuggestions applies persisted page-review suggestions to immutable Markdown snapshots.
package reviewsuggestions

import (
	"slices"
	"sort"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Apply applies non-overlapping new-side line replacements from bottom to top against an immutable source snapshot.
func Apply(markdown string, suggestions []domain.PageReviewComment) (string, error) {
	ordered := slices.Clone(suggestions)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].StartLine != ordered[right].StartLine {
			return ordered[left].StartLine < ordered[right].StartLine
		}
		return ordered[left].EndLine < ordered[right].EndLine
	})

	previousEnd := 0
	for _, suggestion := range ordered {
		if suggestion.Side != domain.PageReviewCommentSideNew || suggestion.StartLine <= previousEnd {
			return "", domain.ErrReviewSuggestionConflict
		}
		original, ok := LineRange(markdown, suggestion.StartLine, suggestion.EndLine)
		if !ok || original != suggestion.Original {
			return "", domain.ErrStaleReview
		}
		previousEnd = suggestion.EndLine
	}

	lines := strings.Split(markdown, "\n")
	for index := len(ordered) - 1; index >= 0; index-- {
		suggestion := ordered[index]
		start := suggestion.StartLine - 1
		end := suggestion.EndLine
		replacement := replacementLines(suggestion.Replacement)

		updated := make([]string, 0, len(lines)-(end-start)+len(replacement))
		updated = append(updated, lines[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[end:]...)
		lines = updated
	}

	return strings.Join(lines, "\n"), nil
}

// LineRange returns an exact one-based inclusive source range without trailing line separators.
func LineRange(markdown string, startLine, endLine int) (string, bool) {
	if startLine <= 0 || endLine < startLine {
		return "", false
	}

	lines := strings.Split(markdown, "\n")
	if startLine > len(lines) || endLine > len(lines) {
		return "", false
	}
	return strings.Join(lines[startLine-1:endLine], "\n"), true
}

// replacementLines converts replacement Markdown into lines while treating an empty replacement as deletion.
func replacementLines(replacement string) []string {
	if replacement == "" {
		return nil
	}
	return strings.Split(replacement, "\n")
}
