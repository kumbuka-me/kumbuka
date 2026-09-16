package auth

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// None authenticates every request as the local administrator.
type None struct {
	// repository persists and resolves the local administrator.
	repository noneRepository
}

// NewNone creates a no-auth authenticator.
func NewNone(repository noneRepository) *None {
	return &None{repository: repository}
}

// Authenticate resolves the local administrator account.
func (a *None) Authenticate(r *http.Request) (domain.User, error) {
	return a.repository.EnsureAdministrator(r.Context(), "admin", "", "Administrator")
}
