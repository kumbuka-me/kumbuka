package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	md "github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/service"
)

// MovePageForm safely moves one page or subtree and optionally refactors direct wiki links.
func MovePageForm(pageUseCases pageMoveService, accessUseCases pageAccessReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid move form.")
			return
		}

		newSlug := md.Slug(r.FormValue("slug"))
		allowed, err := accessUseCases.CanEdit(r.Context(), user, newSlug)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if !allowed {
			httpresponse.Problem(w, http.StatusForbidden, "You do not have permission to move a page to that path.")
			return
		}
		options := domain.MovePageOptions{
			MoveChildren:        r.FormValue("move_children") == "on",
			UpdateIncomingLinks: r.FormValue("update_links") == "on",
			KeepAliases:         r.FormValue("keep_aliases") == "on",
		}
		if err := pageUseCases.Move(r.Context(), r.PathValue("slug"), newSlug, options, user); err != nil {
			writePageProblem(logger, w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+newSlug, http.StatusSeeOther)
	}
}

// ReviewPageForm records an explicit documentation review.
func ReviewPageForm(pageUseCases pageReviewService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		slug := strings.TrimSpace(r.PathValue("slug"))
		if err := pageUseCases.Review(r.Context(), slug, user); err != nil {
			writePageProblem(logger, w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// RequestPageReview opens a lightweight approval request for the current revision.
func RequestPageReview(pageUseCases pageApprovalService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid review request.")
			return
		}

		groupID, ok := reviewGroupID(w, r.FormValue("reviewer_group_id"))
		if !ok {
			return
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		_, err := pageUseCases.RequestReview(r.Context(), service.PageReviewRequestInput{
			Slug:              slug,
			ReviewerUsernames: reviewerUsernames(r.FormValue("reviewers")),
			ReviewerGroupID:   groupID,
			Note:              r.FormValue("note"),
			Actor:             currentUser(r),
		})
		if err != nil {
			writeReviewProblem(w, logger, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// UpdatePageReview edits reviewers or the note of an existing pending request.
func UpdatePageReview(pageUseCases pageApprovalService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid review request.")
			return
		}

		id, ok := reviewRequestID(w, r.PathValue("id"))
		if !ok {
			return
		}
		groupID, ok := reviewGroupID(w, r.FormValue("reviewer_group_id"))
		if !ok {
			return
		}

		slug := strings.TrimSpace(r.FormValue("slug"))
		_, err := pageUseCases.UpdateReview(r.Context(), service.PageReviewUpdateInput{
			ID:                id,
			Slug:              slug,
			ReviewerUsernames: reviewerUsernames(r.FormValue("reviewers")),
			ReviewerGroupID:   groupID,
			Note:              r.FormValue("note"),
			Actor:             currentUser(r),
		})
		if err != nil {
			writeReviewProblem(w, logger, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// CancelPageReview cancels a pending request while preserving its audit history.
func CancelPageReview(pageUseCases pageApprovalService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid review request.")
			return
		}

		id, ok := reviewRequestID(w, r.PathValue("id"))
		if !ok {
			return
		}

		slug := strings.TrimSpace(r.FormValue("slug"))
		if err := pageUseCases.CancelReview(r.Context(), id, slug, currentUser(r)); err != nil {
			writeReviewProblem(w, logger, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// DecidePageReview approves the requested revision or asks for changes.
func DecidePageReview(pageUseCases pageApprovalService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid review decision.")
			return
		}

		id, ok := reviewRequestID(w, r.PathValue("id"))
		if !ok {
			return
		}

		slug := strings.TrimSpace(r.FormValue("slug"))
		err := pageUseCases.DecideReview(r.Context(), service.PageReviewDecisionInput{
			ID:       id,
			Slug:     slug,
			Decision: r.FormValue("decision"),
			Note:     r.FormValue("note"),
			Actor:    currentUser(r),
		})
		if err != nil {
			writeReviewProblem(w, logger, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// reviewRequestID parses the positive review identifier carried by review routes.
func reviewRequestID(w http.ResponseWriter, value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err == nil && id > 0 {
		return id, true
	}

	httpresponse.Problem(w, http.StatusBadRequest, "Invalid review request.")
	return 0, false
}

// reviewGroupID parses the optional reviewer group selected by the request form.
func reviewGroupID(w http.ResponseWriter, value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}

	id, err := strconv.ParseInt(value, 10, 64)
	if err == nil && id > 0 {
		return id, true
	}

	httpresponse.Problem(w,
		http.StatusUnprocessableEntity,
		"Review settings are invalid.",
		httpresponse.NewFieldProblem("reviewer_group_id", "Choose a valid reviewer group."),
	)
	return 0, false
}

// reviewerUsernames parses @username tokens from the compact reviewers field.
func reviewerUsernames(value string) []string {
	parts := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == ';' || character == '\n' || character == '\r' || character == '\t' || character == ' '
	})

	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(part), "@"))
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

// writeReviewProblem translates workflow conflicts before generic page errors.
func writeReviewProblem(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrStaleReview):
		httpresponse.Problem(w, http.StatusConflict, "The page changed after review was requested. Request a new review for the latest revision.")
	case errors.Is(err, domain.ErrReviewPending):
		httpresponse.Problem(w, http.StatusConflict, "This page already has a pending review request.")
	case errors.Is(err, domain.ErrReviewClosed):
		httpresponse.Problem(w, http.StatusConflict, "This review request is already complete and cannot be changed.")
	case errors.Is(err, domain.ErrReviewChangesRequired):
		httpresponse.Problem(w, http.StatusConflict, "Edit the page to address the requested changes before asking for another review.")
	default:
		writePageProblem(logger, w, err)
	}
}
