package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

var (
	// ErrUnauthenticated indicates that an authenticator could not resolve a valid identity.
	ErrUnauthenticated = errors.New("unauthenticated")
	// ErrInvalidCredentials indicates that explicit authentication credentials were supplied but rejected.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrRegistrationDisabled indicates that an authenticated external identity is not allowed to create an account.
	ErrRegistrationDisabled = errors.New("user registration is disabled")
)

// Authenticator resolves an authenticated user from an HTTP request.
type Authenticator interface {
	// Authenticate resolves an authenticated user from the request.
	Authenticate(*http.Request) (domain.User, error)
}

// contextKey is the private request-context key used for authenticated users.
type contextKey struct{}

// User returns the authenticated user stored in the request context.
func User(r *http.Request) (user domain.User, ok bool) {
	user, ok = r.Context().Value(contextKey{}).(domain.User)
	return user, ok
}

// WithUser returns a request whose context contains the authenticated user.
func WithUser(r *http.Request, user domain.User) *http.Request {
	ctx := context.WithValue(r.Context(), contextKey{}, user)
	return r.WithContext(ctx)
}

// ContextUser returns the host-authenticated identity for request-scoped capabilities.
func ContextUser(ctx context.Context) (domain.User, bool) {
	user, ok := ctx.Value(contextKey{}).(domain.User)
	return user, ok
}
