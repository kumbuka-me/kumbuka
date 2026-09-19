package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// PageReview renders the immutable requested revision with line comments and applicable suggestions.
func PageReview(
	viewDataUseCases viewDataService,
	pageUseCases pageApprovalService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reviewID, ok := reviewRequestID(w, r.PathValue("id"))
		if !ok {
			return
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		detail, err := pageUseCases.ReviewDetail(r.Context(), reviewID, slug, currentUser(r))
		if err != nil {
			writeReviewProblem(w, views.Logger(), err)
			return
		}

		data, err := viewDataUseCases.Load(r, views, "Review "+detail.Page.Title)
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		analyzed := revision.Analyze(detail.Revision)
		data.Page = &detail.Page
		data.PageReviewRequest = detail.Request
		data.ReviewDiff = reviewDiffLines(analyzed.Diff, detail.Comments)
		data.CanCommentReview = detail.CanComment
		data.CanSuggestReview = detail.CanSuggest
		data.CanApplyReviewSuggestions = detail.CanApply
		data.OpenReviewSuggestions = openReviewSuggestionCount(detail.Comments)

		views.Render(w, "review", data)
	}
}

// AddPageReviewComment adds line feedback or a Markdown suggestion to a pending review.
func AddPageReviewComment(pageUseCases pageApprovalService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid review comment.")
			return
		}

		reviewID, ok := reviewRequestID(w, r.PathValue("id"))
		if !ok {
			return
		}
		startLine, endLine, ok := reviewLineRange(w, r.FormValue("start_line"), r.FormValue("end_line"))
		if !ok {
			return
		}

		kind := strings.TrimSpace(r.FormValue("kind"))
		if kind != "comment" && kind != "suggestion" {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid review comment type.")
			return
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		comment, err := pageUseCases.AddReviewComment(r.Context(), service.PageReviewCommentInput{
			ReviewID:    reviewID,
			Slug:        slug,
			Side:        strings.TrimSpace(r.FormValue("side")),
			StartLine:   startLine,
			EndLine:     endLine,
			Body:        r.FormValue("body"),
			Suggestion:  kind == "suggestion",
			Replacement: r.FormValue("replacement"),
			Actor:       currentUser(r),
		})
		if err != nil {
			writeReviewProblem(w, views.Logger(), err)
			return
		}

		target := fmt.Sprintf("/reviews/%d/%s#review-comment-%d", reviewID, slug, comment.ID)
		http.Redirect(w, r, target, http.StatusSeeOther)
	}
}

// ApplyPageReviewSuggestion applies one pending suggestion as a new page revision.
func ApplyPageReviewSuggestion(pageUseCases pageApprovalService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reviewID, ok := reviewRequestID(w, r.PathValue("id"))
		if !ok {
			return
		}
		commentID, ok := positiveReviewID(w, r.PathValue("commentID"), "Invalid review suggestion.")
		if !ok {
			return
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		page, err := pageUseCases.ApplyReviewSuggestion(r.Context(), reviewID, slug, commentID, currentUser(r))
		if err != nil {
			writeReviewProblem(w, views.Logger(), err)
			return
		}

		http.Redirect(w, r, "/pages/"+page.Slug, http.StatusSeeOther)
	}
}

// ApplyAllPageReviewSuggestions applies all pending non-overlapping suggestions as one new revision.
func ApplyAllPageReviewSuggestions(pageUseCases pageApprovalService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reviewID, ok := reviewRequestID(w, r.PathValue("id"))
		if !ok {
			return
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		page, err := pageUseCases.ApplyAllReviewSuggestions(r.Context(), reviewID, slug, currentUser(r))
		if err != nil {
			writeReviewProblem(w, views.Logger(), err)
			return
		}

		http.Redirect(w, r, "/pages/"+page.Slug, http.StatusSeeOther)
	}
}

// reviewLineRange parses one positive inclusive source-line range from a review form.
func reviewLineRange(w http.ResponseWriter, startValue, endValue string) (startLine, endLine int, ok bool) {
	startLine, err := strconv.Atoi(strings.TrimSpace(startValue))
	if err != nil || startLine <= 0 {
		httpresponse.Problem(w, http.StatusBadRequest, "Invalid review line.")
		return 0, 0, false
	}

	endValue = strings.TrimSpace(endValue)
	if endValue == "" {
		return startLine, startLine, true
	}
	endLine, err = strconv.Atoi(endValue)
	if err != nil || endLine < startLine {
		httpresponse.Problem(w, http.StatusBadRequest, "Invalid review line range.")
		return 0, 0, false
	}

	return startLine, endLine, true
}

// positiveReviewID parses one positive review-related identifier from a route value.
func positiveReviewID(w http.ResponseWriter, value, message string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err == nil && id > 0 {
		return id, true
	}

	httpresponse.Problem(w, http.StatusBadRequest, message)
	return 0, false
}

// reviewDiffLines joins analyzed diff lines with feedback whose range starts at each anchor.
func reviewDiffLines(diff []revision.DiffLine, comments []domain.PageReviewComment) []webview.ReviewDiffLine {
	lines := make([]webview.ReviewDiffLine, 0, len(diff))
	for _, item := range diff {
		side, line := reviewDiffAnchor(item)
		view := webview.ReviewDiffLine{
			Diff:       item,
			AnchorSide: side,
			AnchorLine: line,
		}

		if line > 0 {
			for _, comment := range comments {
				if comment.Side == side && comment.StartLine == line {
					view.Comments = append(view.Comments, comment)
				}
			}
		}

		lines = append(lines, view)
	}

	return lines
}

// reviewDiffAnchor returns the source side and line used to attach feedback to a diff row.
func reviewDiffAnchor(line revision.DiffLine) (string, int) {
	if line.Kind == "removed" && line.OldLine > 0 {
		return domain.PageReviewCommentSideOld, line.OldLine
	}
	if line.NewLine > 0 {
		return domain.PageReviewCommentSideNew, line.NewLine
	}

	return "", 0
}

// openReviewSuggestionCount returns the number of unapplied suggestions in one review.
func openReviewSuggestionCount(comments []domain.PageReviewComment) int {
	count := 0
	for _, comment := range comments {
		if comment.IsSuggestion && comment.AppliedAt == nil {
			count++
		}
	}

	return count
}
