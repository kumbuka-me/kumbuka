package auth

import (
	"context"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// AuthMode identifies a supported browser authentication mode.
type AuthMode string

const (
	// AuthModeNone authenticates every request as the local administrator.
	AuthModeNone AuthMode = "none"
	// AuthModeLocal authenticates browser requests with Kumbuka-managed credentials.
	AuthModeLocal AuthMode = "local"
	// AuthModeTrustedProxy authenticates users from trusted proxy headers.
	AuthModeTrustedProxy AuthMode = "trusted-proxy"
	// AuthModeOIDC authenticates users through an OIDC provider.
	AuthModeOIDC AuthMode = "oidc"
)

// BrowserConfig contains deployment-level browser authentication configuration.
type BrowserConfig struct {
	// ModeOverride forces one authentication mode for recovery when non-empty.
	ModeOverride AuthMode
	// TrustedProxy contains header overrides used with trusted-proxy recovery mode.
	TrustedProxy TrustedProxyHeaders
	// OIDC contains deployment secrets plus OIDC overrides used with recovery mode.
	OIDC OIDCConfig
	// LocalLoginEnabled exposes local login alongside another configured mode for recovery.
	LocalLoginEnabled bool
}

// OIDCConfig contains the settings required for the OIDC authorization flow.
type OIDCConfig struct {
	ClientID            string
	ClientSecret        string
	Issuer              string
	SessionSecret       string
	PublicURL           string
	GroupClaim          string
	GroupSync           bool
	GroupsAuthoritative bool
	GroupMappings       []domain.OIDCGroupMapping
	AdminGroup          string
}

// BrowserAuth groups dynamic browser identity resolution with its public handlers.
type BrowserAuth struct {
	Authenticator     Authenticator
	Login             http.Handler
	Callback          http.Handler
	Validate          func(context.Context, domain.AuthenticationSettings) error
	Local             *Local
	LocalLoginAllowed func(context.Context) (bool, error)
}
