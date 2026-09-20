package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// unauthorizedReason describes why an authentication attempt was denied.
type unauthorizedReason string

const (
	// unauthorizedCredentialsRequired means no configured authenticator resolved an identity.
	unauthorizedCredentialsRequired unauthorizedReason = "credentials_required"
	// unauthorizedInvalidCredentials means the request explicitly supplied rejected credentials.
	unauthorizedInvalidCredentials unauthorizedReason = "invalid_credentials"
)

// unauthorizedResponder writes the protocol-specific response after authentication is denied.
type unauthorizedResponder func(http.ResponseWriter, *http.Request, unauthorizedReason)

// Authenticate requires browser authentication and redirects unauthenticated users to login.
func Authenticate(logger *slog.Logger, authenticators ...auth.Authenticator) func(http.Handler) http.Handler {
	return authenticate(logger, browserUnauthorized, authenticators...)
}

// AuthenticateAPI requires API authentication and returns JSON for unauthenticated requests.
func AuthenticateAPI(logger *slog.Logger, authenticators ...auth.Authenticator) func(http.Handler) http.Handler {
	return authenticate(logger, apiUnauthorized, authenticators...)
}

// authenticate tries authenticators in order and stores the first resolved user in the request context.
func authenticate(
	logger *slog.Logger,
	responder unauthorizedResponder,
	authenticators ...auth.Authenticator,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, authenticator := range authenticators {
				user, err := authenticator.Authenticate(r)
				if err == nil {
					// The first authenticator that resolves an identity wins.
					// Downstream middleware and handlers can retrieve the user
					// from the request context via auth.User.
					w.Header().Set("X-Kumbuka-User-ID", strconv.FormatInt(user.ID, 10))
					next.ServeHTTP(w, auth.WithUser(r, user))
					return
				}

				switch {
				case errors.Is(err, auth.ErrUnauthenticated):
					// No credentials for this authentication mechanism were
					// present. This is not a failed login; give the next
					// configured authenticator a chance.
					//
					// For example, an API request without a bearer token can
					// still authenticate through its browser session.
					continue

				case errors.Is(err, auth.ErrInvalidCredentials):
					// Credentials for this mechanism were explicitly supplied
					// but rejected. Do not fall back to another authenticator,
					// otherwise invalid credentials could be silently ignored.
					unauthorized(logger, responder, w, r, unauthorizedInvalidCredentials)
					return

				case errors.Is(err, auth.ErrRegistrationDisabled):
					// The external identity is valid, but Kumbuka is not allowed
					// to create the corresponding local user.
					logger.Warn(
						"authentication denied",
						"event", "authentication_denied",
						"reason", "registration_disabled",
						"method", r.Method,
						"path", r.URL.Path,
						"remote_addr", r.RemoteAddr,
					)
					httpresponse.Problem(w, http.StatusForbidden, "User registration is disabled.")
					return

				default:
					// Unexpected authenticator failures are server errors and
					// must not fall through to another authentication mechanism.
					logger.Error(
						"authentication failed",
						"event", "authentication_failed",
						"error", err,
					)
					httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
					return
				}
			}

			// Every authenticator declined the request because none of their
			// supported credentials were present.
			unauthorized(logger, responder, w, r, unauthorizedCredentialsRequired)
		})
	}
}

// unauthorized logs a rejected authentication attempt and writes its protocol-specific response.
func unauthorized(
	logger *slog.Logger,
	responder unauthorizedResponder,
	w http.ResponseWriter,
	r *http.Request,
	reason unauthorizedReason,
) {
	logger.Warn(
		"authentication denied",
		"event", "authentication_denied",
		"reason", string(reason),
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr,
	)
	responder(w, r, reason)
}

// browserUnauthorized redirects an unauthenticated browser request to the login endpoint.
func browserUnauthorized(w http.ResponseWriter, r *http.Request, _ unauthorizedReason) {
	next := url.QueryEscape(browserReturnDestination(r))

	http.Redirect(w, r, "/auth/login?next="+next, http.StatusFound)
}

// browserReturnDestination chooses a readable page to open after authentication.
func browserReturnDestination(r *http.Request) string {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return r.URL.RequestURI()
	}

	// Login resumes with GET, so never return to a mutation-only action URL.
	switch {
	case strings.HasPrefix(r.URL.Path, "/admin/oidc/"),
		strings.HasPrefix(r.URL.Path, "/admin/users/"):
		return "/admin/users"
	default:
		return "/"
	}
}

// apiUnauthorized writes the JSON response for an unauthenticated API request.
func apiUnauthorized(w http.ResponseWriter, _ *http.Request, reason unauthorizedReason) {
	message := "Authentication credentials are required."

	if reason == unauthorizedInvalidCredentials {
		message = "Authorization header must contain a valid Bearer token."
	}

	httpresponse.Problem(w,
		http.StatusUnauthorized,
		"Unauthorized.",
		httpresponse.NewFieldProblem("authorization", message),
	)
}
