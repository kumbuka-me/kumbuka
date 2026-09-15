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

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

const (
	localSessionCookie = "kumbuka_local_session"
	localSessionTTL    = 12 * time.Hour
)

// Local authenticates optional password-backed Kumbuka accounts.
type Local struct {
	repository localRepository
	publicURL  string
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
	cookie, err := r.Cookie(localSessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return domain.User{}, ErrUnauthenticated
	}

	user, err := l.repository.LocalUserBySession(
		r.Context(),
		localSessionHash(cookie.Value),
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

	token, err = newLocalSessionToken()
	if err != nil {
		return domain.User{}, "", err
	}

	if err := l.repository.CreateLocalSession(
		ctx,
		user.ID,
		localSessionHash(token),
		time.Now().Add(localSessionTTL),
	); err != nil {
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

	newHash, err := HashLocalPassword(newPassword)
	if err != nil {
		return "", err
	}

	if err := l.repository.SetLocalCredential(ctx, userID, newHash); err != nil {
		return "", err
	}

	token, err = newLocalSessionToken()
	if err != nil {
		return "", err
	}

	if err := l.repository.CreateLocalSession(
		ctx,
		userID,
		localSessionHash(token),
		time.Now().Add(localSessionTTL),
	); err != nil {
		return "", err
	}

	return token, nil
}

// Setup creates the first local administrator and starts its initial session.
func (l *Local) Setup(
	ctx context.Context,
	username, email, displayName, password string,
) (user domain.User, token string, err error) {
	passwordHash, err := HashLocalPassword(password)
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

	token, err = newLocalSessionToken()
	if err != nil {
		return domain.User{}, "", err
	}

	if err := l.repository.CreateLocalSession(
		ctx,
		user.ID,
		localSessionHash(token),
		time.Now().Add(localSessionTTL),
	); err != nil {
		return domain.User{}, "", err
	}

	return user, token, nil
}

// SetPassword creates or replaces one Kumbuka user's local recovery password.
func (l *Local) SetPassword(ctx context.Context, userID int64, password string) error {
	passwordHash, err := HashLocalPassword(password)
	if err != nil {
		return err
	}

	return l.repository.SetLocalCredential(ctx, userID, passwordHash)
}

// WriteSessionCookie stores a local session token in an HTTP-only cookie.
func (l *Local) WriteSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     localSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(localSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   strings.HasPrefix(l.publicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSession revokes the current local session and removes its cookie.
func (l *Local) ClearSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(localSessionCookie); err == nil && cookie.Value != "" {
		_ = l.repository.DeleteLocalSession(
			r.Context(),
			localSessionHash(cookie.Value),
		)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     localSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(l.publicURL, "https://"),
		SameSite: http.SameSiteLaxMode,
	})
}

// HashLocalPassword hashes a validated local password with bcrypt.
func HashLocalPassword(password string) (string, error) {
	if problem := LocalPasswordProblem(password); problem != "" {
		return "", domain.NewValidationError("password", problem)
	}

	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)

	return string(hash), err
}

// newLocalSessionToken creates an opaque random browser-session token.
func newLocalSessionToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(data), nil
}

// localSessionHash returns the at-rest representation of a local session token.
func localSessionHash(token string) string {
	hash := sha256.Sum256([]byte(token))

	return hex.EncodeToString(hash[:])
}
