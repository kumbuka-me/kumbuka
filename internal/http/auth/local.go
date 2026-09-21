package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/credential"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"golang.org/x/crypto/bcrypt"
)

const (
	localSessionCookie     = "kumbuka_local_session"
	bootstrapSessionCookie = "kumbuka_bootstrap_session"
	localSessionTTL        = 12 * time.Hour
)

// Local authenticates optional password-backed Kumbuka accounts.
type Local struct {
	// repository persists credentials and local browser sessions.
	repository localRepository
	// publicURL determines whether browser cookies require HTTPS.
	publicURL string
}

// NewLocal creates the local-login authenticator used by setup and recovery login.
func NewLocal(repository localRepository, publicURL string) *Local {
	return &Local{
		repository: repository,
		publicURL:  strings.TrimSpace(publicURL),
	}
}

// Authenticate resolves a valid local session cookie.
func (l *Local) Authenticate(r *http.Request) (domain.User, error) {
	return l.authenticateSession(r, localSessionCookie, localSessionHash)
}

// authenticateBootstrapSession resolves the setup-only session used while a runtime authentication override is active.
func (l *Local) authenticateBootstrapSession(r *http.Request) (domain.User, error) {
	return l.authenticateSession(r, bootstrapSessionCookie, bootstrapSessionHash)
}

// authenticateSession resolves one valid local session cookie with its purpose-specific token hash.
func (l *Local) authenticateSession(
	r *http.Request,
	cookieName string,
	hashToken func(string) string,
) (domain.User, error) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return domain.User{}, ErrUnauthenticated
	}

	user, err := l.repository.LocalUserBySession(
		r.Context(),
		hashToken(cookie.Value),
	)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && !user.Enabled) {
		return domain.User{}, ErrUnauthenticated
	}

	return user, err
}

// SignIn verifies local credentials and creates a new browser session.
func (l *Local) SignIn(
	ctx context.Context,
	username, password string,
) (user domain.User, token string, err error) {
	user, passwordHash, err := l.repository.LocalCredential(
		ctx,
		strings.TrimSpace(username),
	)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && !user.Enabled) {
		return domain.User{}, "", ErrInvalidCredentials
	}
	if err != nil {
		return domain.User{}, "", err
	}

	if bcrypt.CompareHashAndPassword(
		[]byte(passwordHash),
		[]byte(password),
	) != nil {
		return domain.User{}, "", ErrInvalidCredentials
	}

	token, err = l.createSession(ctx, user.ID, localSessionHash)
	if err != nil {
		return domain.User{}, "", err
	}

	return user, token, nil
}

// ChangePassword verifies the current credential, replaces it, and creates a fresh session.
func (l *Local) ChangePassword(
	ctx context.Context,
	userID int64,
	username, currentPassword, newPassword string,
) (token string, err error) {
	user, passwordHash, err := l.repository.LocalCredential(
		ctx,
		strings.TrimSpace(username),
	)
	if errors.Is(err, domain.ErrNotFound) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}
	if !user.Enabled || user.ID != userID {
		return "", ErrInvalidCredentials
	}

	if bcrypt.CompareHashAndPassword(
		[]byte(passwordHash),
		[]byte(currentPassword),
	) != nil {
		return "", ErrInvalidCredentials
	}

	newHash, err := credential.HashLocalPassword(newPassword)
	if err != nil {
		return "", err
	}

	if err := l.repository.SetLocalCredential(ctx, userID, newHash); err != nil {
		return "", err
	}

	return l.createSession(ctx, userID, localSessionHash)
}

// Setup creates the first local administrator and starts its initial local session.
func (l *Local) Setup(
	ctx context.Context,
	username, email, displayName, password string,
) (user domain.User, token string, err error) {
	return l.setup(ctx, username, email, displayName, password, localSessionHash)
}

// SetupBootstrap creates the first local administrator and a setup-only session for deployments with an authentication override.
func (l *Local) SetupBootstrap(
	ctx context.Context,
	username, email, displayName, password string,
) (user domain.User, token string, err error) {
	return l.setup(ctx, username, email, displayName, password, bootstrapSessionHash)
}

// setup creates the first local administrator and a purpose-bound browser session.
func (l *Local) setup(
	ctx context.Context,
	username, email, displayName, password string,
	hashToken func(string) string,
) (user domain.User, token string, err error) {
	passwordHash, err := credential.HashLocalPassword(password)
	if err != nil {
		return domain.User{}, "", err
	}

	user, err = l.repository.CreateInitialLocalAdministrator(
		ctx,
		username,
		email,
		displayName,
		passwordHash,
	)
	if err != nil {
		return domain.User{}, "", err
	}

	token, err = l.createSession(ctx, user.ID, hashToken)
	if err != nil {
		return domain.User{}, "", err
	}

	return user, token, nil
}

// WriteSessionCookie stores a local session token in an HTTP-only cookie.
func (l *Local) WriteSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, l.sessionCookie(localSessionCookie, token, int(localSessionTTL.Seconds())))
}

// WriteBootstrapSessionCookie stores a setup-only session token in an HTTP-only cookie.
func (l *Local) WriteBootstrapSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, l.sessionCookie(bootstrapSessionCookie, token, int(localSessionTTL.Seconds())))
}

// ClearSession revokes current local and bootstrap sessions and removes their cookies.
func (l *Local) ClearSession(w http.ResponseWriter, r *http.Request) {
	l.clearSessionCookie(w, r, localSessionCookie, localSessionHash)
	l.clearSessionCookie(w, r, bootstrapSessionCookie, bootstrapSessionHash)
}

// clearSessionCookie revokes one purpose-bound session when present and expires its browser cookie.
func (l *Local) clearSessionCookie(
	w http.ResponseWriter,
	r *http.Request,
	cookieName string,
	hashToken func(string) string,
) {
	if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
		_ = l.repository.DeleteLocalSession(
			r.Context(),
			hashToken(cookie.Value),
		)
	}

	http.SetCookie(w, l.sessionCookie(cookieName, "", -1))
}

// createSession creates and persists one opaque local browser session.
func (l *Local) createSession(
	ctx context.Context,
	userID int64,
	hashToken func(string) string,
) (string, error) {
	token, err := newLocalSessionToken()
	if err != nil {
		return "", err
	}

	if err := l.repository.CreateLocalSession(
		ctx,
		userID,
		hashToken(token),
		time.Now().Add(localSessionTTL),
	); err != nil {
		return "", err
	}

	return token, nil
}

// sessionCookie builds a local-session cookie with the application's shared security attributes.
func (l *Local) sessionCookie(name, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   strings.HasPrefix(l.publicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	}
}

// newLocalSessionToken creates an opaque random browser-session token.
func newLocalSessionToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}

// localSessionHash returns the at-rest representation of a regular local session token.
func localSessionHash(token string) string {
	hash := sha256.Sum256([]byte(token))

	return hex.EncodeToString(hash[:])
}

// bootstrapSessionHash returns a purpose-bound at-rest representation of a setup bootstrap session token.
func bootstrapSessionHash(token string) string {
	hash := sha256.Sum256([]byte("bootstrap\x00" + token))

	return hex.EncodeToString(hash[:])
}
