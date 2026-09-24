package endpoint

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// PageReview renders the immutable requested revision with line comments and applicable suggestions.
func PageReview(
	browserContext browserContextLoader,
	pageUseCases pageReviewDiscussionService,
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

		layout, err := browserContext.Load(r, views, "Review "+detail.Page.Title)
		data := webview.ReviewView{Layout: layout}
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		analyzed := revision.Analyze(detail.Revision)
		data.Page = &detail.Page
		data.PageReviewRequest = detail.Request
		data.ReviewDiff = webview.ReviewDiffLines(analyzed.Diff, detail.Comments)
		data.CanCommentReview = detail.CanComment
		data.CanSuggestReview = detail.CanSuggest
		data.CanApplyReviewSuggestions = detail.CanApply
		data.OpenReviewSuggestions = webview.OpenReviewSuggestionCount(detail.Comments)

		data.CurrentPage = webview.CurrentPage(data.Page)
		views.Render(w, "review", data)
	}
}

// AddPageReviewComment adds line feedback or a Markdown suggestion to a pending review.
func AddPageReviewComment(pageUseCases pageReviewDiscussionService, views *webview.Views) http.HandlerFunc {
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
		comment, err := pageUseCases.AddReviewComment(r.Context(), apppages.PageReviewCommentInput{
			ReviewID:    reviewID,
			Slug:        slug,
			Side:        domain.PageReviewCommentSide(strings.TrimSpace(r.FormValue("side"))),
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
func ApplyPageReviewSuggestion(pageUseCases pageReviewDiscussionService, views *webview.Views) http.HandlerFunc {
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
func ApplyAllPageReviewSuggestions(pageUseCases pageReviewDiscussionService, views *webview.Views) http.HandlerFunc {
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
