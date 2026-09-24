package webview

import (
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// PartitionPageComments separates page-level discussion from anchored inline threads.
func PartitionPageComments(comments []domain.PageComment) ([]domain.PageComment, []PageCommentThread) {
	byID := make(map[int64]domain.PageComment, len(comments))
	for _, comment := range comments {
		byID[comment.ID] = comment
	}

	pageComments := make([]domain.PageComment, 0, len(comments))
	inlineThreads := make([]PageCommentThread, 0)
	threadIndex := make(map[int64]int)

	for _, comment := range comments {
		root := pageCommentRoot(comment, byID)
		if strings.TrimSpace(root.Anchor) == "" {
			pageComments = append(pageComments, comment)
			continue
		}

		index, ok := threadIndex[root.ID]
		if !ok {
			index = len(inlineThreads)
			threadIndex[root.ID] = index
			inlineThreads = append(inlineThreads, PageCommentThread{
				RootID:   root.ID,
				Anchor:   root.Anchor,
				Resolved: root.Resolved != nil,
				Comments: []domain.PageComment{root},
			})
		}

		if comment.ID == root.ID {
			continue
		}

		inlineThreads[index].Comments = append(inlineThreads[index].Comments, comment)
	}

	return pageComments, inlineThreads
}

// pageCommentRoot resolves the root comment for one reply chain without trusting malformed cycles.
func pageCommentRoot(comment domain.PageComment, byID map[int64]domain.PageComment) domain.PageComment {
	seen := make(map[int64]bool)
	current := comment

	for current.ParentID > 0 && !seen[current.ID] {
		seen[current.ID] = true

		parent, ok := byID[current.ParentID]
		if !ok {
			break
		}

		current = parent
	}

	return current
}

// reviewCommentAnchor identifies one side and source line in a page review diff.
type reviewCommentAnchor struct {
	// side is old or new source content.
	side domain.PageReviewCommentSide
	// line is the one-based source line number.
	line int
}

// ReviewDiffLines joins analyzed diff lines with feedback whose range starts at each anchor.
func ReviewDiffLines(diff []revision.DiffLine, comments []domain.PageReviewComment) []ReviewDiffLine {
	commentsByAnchor := make(map[reviewCommentAnchor][]domain.PageReviewComment, len(comments))
	for _, comment := range comments {
		key := reviewCommentAnchorKey(comment.Side, comment.StartLine)
		commentsByAnchor[key] = append(commentsByAnchor[key], comment)
	}

	lines := make([]ReviewDiffLine, 0, len(diff))
	for _, item := range diff {
		side, line := reviewDiffAnchor(item)
		view := ReviewDiffLine{
			Diff:       item,
			AnchorSide: side,
			AnchorLine: line,
		}
		if line > 0 {
			view.Comments = commentsByAnchor[reviewCommentAnchorKey(side, line)]
		}

		lines = append(lines, view)
	}

	return lines
}

// reviewDiffAnchor returns the source side and line used to attach feedback to a diff row.
func reviewDiffAnchor(line revision.DiffLine) (domain.PageReviewCommentSide, int) {
	if line.Kind == revision.DiffLineRemoved && line.OldLine > 0 {
		return domain.PageReviewCommentSideOld, line.OldLine
	}
	if line.NewLine > 0 {
		return domain.PageReviewCommentSideNew, line.NewLine
	}

	return "", 0
}

// reviewCommentAnchorKey returns a stable map key for one review source position.
func reviewCommentAnchorKey(side domain.PageReviewCommentSide, line int) reviewCommentAnchor {
	return reviewCommentAnchor{side: side, line: line}
}

// OpenReviewSuggestionCount returns the number of unapplied suggestions in one review.
func OpenReviewSuggestionCount(comments []domain.PageReviewComment) int {
	count := 0
	for _, comment := range comments {
		if comment.IsSuggestion && comment.AppliedAt == nil {
			count++
		}
	}

	return count
}
