package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// CreateSavedSearch creates a personal smart collection.
func CreateSavedSearch(knowledgeUseCases savedSearchService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid saved search form.")
			return
		}
		if err := knowledgeUseCases.SaveSavedSearch(
			r.Context(),
			user.ID,
			0,
			r.FormValue("name"),
			r.FormValue("query"),
			r.FormValue("pinned") == "on",
		); err != nil {
			writeSavedSearchProblem(logger, w, err)
			return
		}

		next := strings.TrimSpace(r.FormValue("next"))
		if !httpresponse.IsLocalPath(next) {
			next = "/settings#saved-searches"
		}

		http.Redirect(w, r, next, http.StatusSeeOther)
	}
}

// DeleteSavedSearch deletes one personal smart collection.
func DeleteSavedSearch(knowledgeUseCases savedSearchService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid saved search.")
			return
		}
		if err := knowledgeUseCases.DeleteSavedSearch(r.Context(), user.ID, id); err != nil {
			writeSavedSearchProblem(logger, w, err)
			return
		}

		http.Redirect(w, r, "/settings#saved-searches", http.StatusSeeOther)
	}
}

// writeSavedSearchProblem translates failures for searches owned by the current user.
func writeSavedSearchProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	if tryWriteValidationProblem(w, err, "Saved search validation failed.") {
		return
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w, http.StatusNotFound, "Saved search not found.")
	case errors.Is(err, domain.ErrAlreadyExists):
		httpresponse.Problem(w, http.StatusConflict, "Saved search already exists.")
	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}
