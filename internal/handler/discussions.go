package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// AddPageComment adds an anchored discussion item to one page.
func AddPageComment(pageUseCases pageDiscussionWriter, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid comment form.")
			return
		}

		var parentID int64
		if rawParentID := strings.TrimSpace(r.FormValue("parent_id")); rawParentID != "" {
			parsedParentID, err := strconv.ParseInt(rawParentID, 10, 64)
			if err != nil || parsedParentID <= 0 {
				httpresponse.Problem(w, http.StatusBadRequest, "Invalid reply target.")
				return
			}

			parentID = parsedParentID
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		comment, err := pageUseCases.AddComment(
			r.Context(), slug, parentID, r.FormValue("anchor"), r.FormValue("quote"), r.FormValue("body"), user,
		)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug+"#comment-"+strconv.FormatInt(comment.ID, 10), http.StatusSeeOther)
	}
}

// ResolvePageComment resolves or reopens one discussion item.
func ResolvePageComment(pageUseCases pageDiscussionWriter, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid comment.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid comment form.")
			return
		}
		if err := pageUseCases.ResolveComment(r.Context(), id, r.FormValue("resolved") != "false"); err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		next := strings.TrimSpace(r.FormValue("next"))
		if next == "" || !strings.HasPrefix(next, "/pages/") {
			next = "/"
		}

		http.Redirect(w, r, next+"#comments", http.StatusSeeOther)
	}
}
