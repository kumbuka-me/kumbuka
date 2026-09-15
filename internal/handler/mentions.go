package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
)

// mentionUser contains the account information exposed by the mention picker.
type mentionUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Self        bool   `json:"self"`
}

// MentionUsers returns accounts matching an editor mention query.
func MentionUsers(userUseCases userDirectoryService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}

		users, err := userUseCases.SearchUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), 50)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		result := make([]mentionUser, 0, len(users))

		for _, user := range users {
			if !user.Enabled {
				continue
			}

			result = append(result, mentionUser{
				Username:    user.Username,
				DisplayName: user.DisplayName,
				Role:        user.Role,
				Self:        user.ID == current.ID,
			})
		}

		httpresponse.Respond(w, http.StatusOK, result)
	}
}
