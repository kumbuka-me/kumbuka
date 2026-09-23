package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Bearer authenticates API requests using bearer tokens.
type Bearer struct {
	// repository resolves API tokens to Kumbuka users.
	repository bearerRepository
}

// NewBearer creates a bearer-token authenticator.
func NewBearer(repository bearerRepository) *Bearer {
	return &Bearer{repository: repository}
}

// Authenticate resolves the bearer token in the Authorization header.
func (a *Bearer) Authenticate(r *http.Request) (domain.User, error) {
	value, supplied := r.Header["Authorization"]
	if !supplied {
		return domain.User{}, ErrUnauthenticated
	}

	token, ok := parseBearerValue(strings.Join(value, ","))
	if !ok {
		return domain.User{}, ErrInvalidCredentials
	}

	user, err := a.repository.UserByToken(r.Context(), token)
	if unavailableAuthenticatedUser(user, err) {
		return domain.User{}, ErrInvalidCredentials
	}

	return user, err
}

// unavailableAuthenticatedUser reports whether an authentication lookup found no usable enabled user.
func unavailableAuthenticatedUser(user domain.User, err error) bool {
	return errors.Is(err, domain.ErrNotFound) || err == nil && !user.Enabled
}

// parseBearerValue validates an explicit Authorization header and returns its bearer token.
func parseBearerValue(value string) (token string, ok bool) {
	fields := strings.Fields(value)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return "", false
	}

	token = strings.TrimSpace(fields[1])

	return token, token != ""
}
