package handler

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// currentUser returns the user populated by route authentication middleware.
func currentUser(r *http.Request) domain.User {
	user, _ := auth.User(r)
	return user
}
