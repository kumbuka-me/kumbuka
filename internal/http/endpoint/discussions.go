package endpoint

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// AddPageComment adds a page discussion comment or an applicable inline suggestion.
func AddPageComment(pageUseCases pageDiscussionWriter, views *webview.Views,
	accessUseCases pageAccessReader,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizePageRequest(w, r, accessUseCases, false) {
			return
		}
		user, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid comment form.")
			return
		}

		parentID, ok := pageCommentParentID(w, r.FormValue("parent_id"))
		if !ok {
			return
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		kind := strings.TrimSpace(r.FormValue("kind"))
		if kind == "suggestion" {
			if parentID != 0 || strings.TrimSpace(r.FormValue("quote")) != "" {
				httpresponse.Problem(w, http.StatusBadRequest, "Suggestions must start a new inline discussion.")
				return
			}

			comment, err := pageUseCases.AddSuggestion(
				r.Context(),
				slug,
				r.FormValue("anchor"),
				r.FormValue("body"),
				r.FormValue("replacement"),
				user,
			)
			if err != nil {
				writePageProblem(views.Logger(), w, err)
				return
			}

			http.Redirect(w, r, pageCommentTarget(slug, comment.ID), http.StatusSeeOther)
			return
		}
		if kind != "" && kind != "comment" {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid discussion type.")
			return
		}

		comment, err := pageUseCases.AddComment(
			r.Context(), slug, parentID, r.FormValue("anchor"), r.FormValue("quote"), r.FormValue("body"), user,
		)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, pageCommentTarget(slug, comment.ID), http.StatusSeeOther)
	}
}

// ApplyPageCommentSuggestion applies one inline suggestion and creates a new page revision.
func ApplyPageCommentSuggestion(pageUseCases pageDiscussionWriter, views *webview.Views,
	accessUseCases pageAccessReader,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizePageRequest(w, r, accessUseCases, true) {
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(r.PathValue("id")), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid inline suggestion.")
			return
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		page, err := pageUseCases.ApplyCommentSuggestion(r.Context(), slug, id, currentUser(r))
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, pageCommentTarget(page.Slug, id), http.StatusSeeOther)
	}
}

// ResolvePageComment resolves or reopens one discussion item.
func ResolvePageComment(pageUseCases pageDiscussionWriter, views *webview.Views,
	accessUseCases pageAccessReader,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorizePageRequest(w, r, accessUseCases, true) {
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid comment.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid comment form.")
			return
		}
		slug := strings.TrimSpace(r.PathValue("slug"))
		if err := pageUseCases.ResolveComment(r.Context(), slug, id, r.FormValue("resolved") != "false"); err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		next := pageCommentReturnTarget(r.FormValue("next"))
		http.Redirect(w, r, next, http.StatusSeeOther)
	}
}

// pageCommentParentID parses an optional positive reply target from a discussion form.
func pageCommentParentID(w http.ResponseWriter, value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, true
	}

	parentID, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parentID <= 0 {
		httpresponse.Problem(w, http.StatusBadRequest, "Invalid reply target.")
		return 0, false
	}

	return parentID, true
}

// pageCommentTarget returns the page URL that reopens one discussion item after a mutation.
func pageCommentTarget(slug string, id int64) string {
	return "/pages/" + strings.TrimSpace(slug) + "#comment-" + strconv.FormatInt(id, 10)
}

// pageCommentReturnTarget preserves an inline-comment fragment while defaulting page discussions to their section.
func pageCommentReturnTarget(value string) string {
	target := strings.TrimSpace(value)
	if !httpresponse.IsLocalPath(target) || !strings.HasPrefix(target, "/pages/") {
		return "/"
	}
	if strings.Contains(target, "#") {
		return target
	}

	return target + "#comments"
}
