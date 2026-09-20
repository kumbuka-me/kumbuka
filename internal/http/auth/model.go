package auth

import (
	"context"
	"net/http"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// BrowserConfig contains deployment-level browser authentication configuration.
type BrowserConfig struct {
	// ModeOverride forces one authentication mode for recovery when non-empty.
	ModeOverride domain.AuthMode
	// TrustedProxy contains header overrides used with trusted-proxy recovery mode.
	TrustedProxy TrustedProxyHeaders
	// OIDC contains deployment secrets plus OIDC overrides used with recovery mode.
	OIDC OIDCConfig
	// LocalLoginEnabled exposes local login alongside another configured mode for recovery.
	LocalLoginEnabled bool
}

// OIDCConfig contains the settings required for the OIDC authorization flow.
type OIDCConfig struct {
	// ClientID identifies Kumbuka to the OIDC provider.
	ClientID string
	// ClientSecret authenticates Kumbuka during authorization-code exchange.
	ClientSecret string
	// Issuer is the expected OIDC provider issuer URL.
	Issuer string
	// SessionSecret signs browser login-state and session cookies.
	SessionSecret string
	// PublicURL is the externally visible Kumbuka base URL used for callbacks and cookies.
	PublicURL string
	// GroupClaim names the identity-token claim containing external group memberships.
	GroupClaim string
	// GroupSync enables synchronization of mapped OIDC groups at login.
	GroupSync bool
	// GroupsAuthoritative removes mapped memberships missing from the current identity token.
	GroupsAuthoritative bool
	// GroupMappings maps external OIDC group names to Kumbuka group IDs.
	GroupMappings []domain.OIDCGroupMapping
	// AdminGroup names the external group that grants administrator access for the session.
	AdminGroup string
}

// BrowserAuth groups dynamic browser identity resolution with its public handlers.
type BrowserAuth struct {
	// Authenticator resolves the current browser identity.
	Authenticator Authenticator
	// Login starts the interactive login flow for the effective authentication mode.
	Login http.Handler
	// Callback completes the OIDC login flow when OIDC is active.
	Callback http.Handler
	// Validate checks proposed persisted authentication settings.
	Validate func(context.Context, domain.AuthenticationSettings) error
	// Local provides local setup, recovery login, and password-management operations.
	Local *Local
	// LocalLoginAllowed reports whether the local sign-in endpoint is currently available.
	LocalLoginAllowed func(context.Context) (bool, error)
}
