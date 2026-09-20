package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// NotificationsAPI returns the current user's notification inbox.
func NotificationsAPI(notificationUseCases notificationService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}

		items, unread, err := notificationUseCases.Notifications(r.Context(), user.ID, 30)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		w.Header().Set("Cache-Control", "private, no-store")
		httpresponse.Respond(w, http.StatusOK, map[string]any{"items": jsonSlice(items), "unread": unread})
	}
}

// OpenNotification marks an owned notification read before redirecting to its stored destination.
func OpenNotification(notificationUseCases notificationService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}

		id, err := notificationID(r.PathValue("id"))
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid notification.")
			return
		}

		destination, err := notificationUseCases.OpenNotification(r.Context(), user.ID, id)
		if errors.Is(err, domain.ErrNotFound) {
			httpresponse.Problem(w, http.StatusNotFound, "Notification not found.")
			return
		}
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if !httpresponse.IsLocalPath(destination) {
			destination = "/"
		}

		w.Header().Set("Cache-Control", "private, no-store")
		http.Redirect(w, r, destination, http.StatusSeeOther)
	}
}

// MarkNotificationRead marks one notification or the complete inbox as read.
func MarkNotificationRead(notificationUseCases notificationService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}

		value := r.PathValue("id")
		if value == "all" {
			if err := notificationUseCases.MarkAllNotificationsRead(r.Context(), user.ID); err != nil {
				httpresponse.InternalServerError(logger, w, err)
				return
			}
		} else {
			id, err := notificationID(value)
			if err != nil {
				httpresponse.Problem(w, http.StatusBadRequest, "Invalid notification.")
				return
			}
			if err := notificationUseCases.MarkNotificationRead(r.Context(), user.ID, id); err != nil {
				httpresponse.InternalServerError(logger, w, err)
				return
			}
		}

		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(http.StatusNoContent)
	}
}

// notificationID parses one positive notification identifier.
func notificationID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid notification id")
	}

	return id, nil
}
