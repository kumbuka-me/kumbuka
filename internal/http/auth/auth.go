package auth

import (
	"context"
	"errors"
	"net/http"

	appkitauth "github.com/containeroo/appkit/auth"
	appkithttpauth "github.com/containeroo/appkit/httpauth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

var (
	// ErrUnauthenticated indicates that an authenticator could not resolve a valid identity.
	ErrUnauthenticated = appkitauth.ErrUnauthenticated
	// ErrInvalidCredentials indicates that explicit authentication credentials were supplied but rejected.
	ErrInvalidCredentials = appkitauth.ErrInvalidCredentials
	// ErrRegistrationDisabled indicates that an authenticated external identity is not allowed to create an account.
	ErrRegistrationDisabled = errors.New("user registration is disabled")
)

// Authenticator resolves an authenticated user from an HTTP request.
type Authenticator = appkitauth.Authenticator[domain.User]

// User returns the authenticated user stored in the request context.
func User(r *http.Request) (domain.User, bool) {
	return appkithttpauth.CurrentUser[domain.User](r)
}

// WithUser returns a request whose context contains the authenticated user.
func WithUser(r *http.Request, user domain.User) *http.Request {
	return appkithttpauth.WithUser(r, user)
}

// ContextUser returns the host-authenticated identity for request-scoped capabilities.
func ContextUser(ctx context.Context) (domain.User, bool) {
	return appkithttpauth.CurrentUserFromContext[domain.User](ctx)
}
