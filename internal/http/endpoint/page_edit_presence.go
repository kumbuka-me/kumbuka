package endpoint

import (
	"log/slog"
	"net/http"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// PageEditors returns other users who are currently editing the requested page.
func PageEditors(presence pagePresenceService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		editors, err := presence.PageEditors(r.Context(), r.PathValue("slug"), currentUser(r))
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, map[string]any{"editors": jsonSlice(editors)})
	}
}

// TouchPageEditor refreshes the current user's active editor presence.
func TouchPageEditor(presence pagePresenceService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := presence.TouchPageEditor(r.Context(), r.PathValue("slug"), currentUser(r)); err != nil {
			writePageProblem(logger, w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// LeavePageEditor clears the current user's active editor presence immediately.
func LeavePageEditor(presence pagePresenceService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := presence.LeavePageEditor(r.Context(), r.PathValue("slug"), currentUser(r)); err != nil {
			writePageProblem(logger, w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
