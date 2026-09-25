package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// integrationUser contains contact data exposed only to enabled administrator integrations.
type integrationUser struct {
	// ID is the stable Kumbuka user identifier.
	ID int64 `json:"id"`
	// Mention is the canonical @-prefixed username.
	Mention string `json:"mention"`
	// DisplayName is the human-readable account name.
	DisplayName string `json:"display_name"`
	// Email is the account contact email.
	Email string `json:"email"`
	// Enabled reports whether the account can currently receive integration delivery.
	Enabled bool `json:"enabled"`
}

// IntegrationUser returns one user's contact profile when the administrator-controlled API is enabled.
func IntegrationUser(users userDirectoryService, settings settingsService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		configuration, err := settings.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if !configuration.IntegrationUserDirectoryEnabled {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid user.")
			return
		}
		user, err := users.User(r.Context(), id)
		if errors.Is(err, domain.ErrNotFound) {
			httpresponse.Problem(w, http.StatusNotFound, "User not found.")
			return
		}
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		httpresponse.Respond(w, http.StatusOK, integrationUser{
			ID:          user.ID,
			Mention:     "@" + user.Username,
			DisplayName: user.DisplayName,
			Email:       user.Email,
			Enabled:     user.Enabled,
		})
	}
}
