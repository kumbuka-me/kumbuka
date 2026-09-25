package endpoint

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// mentionDirectoryService exposes privacy-safe user searches for editor autocomplete.
type mentionDirectoryService interface {
	SearchPublicUsers(context.Context, string, int) ([]domain.User, error)
}

// mentionUser contains the account information exposed by the mention picker.
type mentionUser struct {
	// Username is the username associated with mention user.
	Username string `json:"username"`
	// DisplayName is the display name associated with mention user.
	DisplayName string `json:"display_name"`
	// Role is the role associated with mention user.
	Role domain.UserRole `json:"role"`
	// Self reports whether self applies to mention user.
	Self bool `json:"self"`
}

// MentionUsers returns accounts matching an editor mention query.
func MentionUsers(userUseCases mentionDirectoryService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		current, ok := auth.User(r)
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "Unauthorized.")
			return
		}

		users, err := userUseCases.SearchPublicUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), 50)
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
