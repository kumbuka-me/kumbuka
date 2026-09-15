// Package serve configures and runs the database-backed Kumbuka server.
package serve

import (
	"errors"
	"net"

	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/logging"
	"github.com/kumbuka-me/kumbuka/internal/pdf"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
)

var trustedUsernameHeaders = []string{
	"X-Forwarded-User",
	"X-Auth-Request-User",
	"Remote-User",
}

var trustedEmailHeaders = []string{
	"X-Forwarded-Email",
	"X-Auth-Request-Email",
	"X-Authentik-Email",
}

var trustedDisplayNameHeaders = []string{
	"X-Forwarded-Name",
	"X-Auth-Request-Preferred-Username",
	"X-Authentik-Name",
}

// Config contains deployment-level runtime configuration for Kumbuka.
type Config struct {
	// ListenAddress is the TCP address used by the HTTP server.
	ListenAddress string
	// DatabaseURL is the PostgreSQL connection URL.
	DatabaseURL string
	// PublicURL is the externally visible base URL.
	PublicURL string
	// PDFURL optionally overrides the persisted PDF rendering endpoint for this process.
	PDFURL string
	// AuthModeOverride forces one browser authentication mode for recovery when non-empty.
	AuthModeOverride auth.AuthMode
	// TrustedUsernameHeaders are used only by the trusted-proxy runtime override.
	TrustedUsernameHeaders []string
	// TrustedEmailHeaders are used only by the trusted-proxy runtime override.
	TrustedEmailHeaders []string
	// TrustedDisplayNameHeaders are used only by the trusted-proxy runtime override.
	TrustedDisplayNameHeaders []string
	// OIDCIssuer is used only by the OIDC runtime override.
	OIDCIssuer string
	// OIDCClientID is used only by the OIDC runtime override.
	OIDCClientID string
	// OIDCClientSecret is the deployment-managed OIDC client secret.
	OIDCClientSecret string
	// OIDCSessionSecret signs OIDC login and session cookies.
	OIDCSessionSecret string
	// EncryptionKey encrypts sensitive application settings stored in PostgreSQL.
	EncryptionKey string
	// LocalLogin enables the local recovery login alongside the configured mode.
	LocalLogin bool
	// ThemeDirectory optionally overlays embedded themes with TOML files from disk.
	ThemeDirectory string
	// LogFormat selects structured text or JSON logging.
	LogFormat logging.LogFormat
	// Debug enables verbose diagnostic logging.
	Debug bool
	// DebugRenderTimings enables detailed page handler, Markdown, and WASM timing logs.
	DebugRenderTimings bool
	// AccessLog enables HTTP request logging.
	AccessLog bool
}

// BindFlags registers the kumbuka serve flags and returns a resolver for the parsed Config.
func BindFlags(flags *tinyflags.FlagSet) func() Config {
	cfg := Config{}

	// Server
	listen := flags.TCPAddr("listen-address", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}, "Address on which the web server listens").
		Short("a").
		Placeholder("ADDR").
		Value()

	flags.StringVar(&cfg.DatabaseURL, "database-url", "", "PostgreSQL connection URL").
		Required().
		Placeholder("URL").
		OverriddenValueMaskFn(tinyflags.MaskPostgresURL).
		Value()
	flags.StringVar(&cfg.PublicURL, "public-url", "http://localhost:8080", "Externally visible base URL").
		Placeholder("URL").
		Value()
	flags.StringVar(&cfg.PDFURL, "pdf-url", "", "Deployment override for the PDF service POST URL, including its path").
		Placeholder("URL").
		Validate(pdf.ValidateURL).
		Value()
	flags.BoolVar(&cfg.LocalLogin, "local-login", false, "Enable the local recovery login alongside the configured authentication mode").
		Value()
	flags.StringVar(&cfg.ThemeDirectory, "theme-directory", "", "Directory containing custom theme TOML files that override or extend embedded themes").
		Placeholder("DIR").
		Value()

	// Auth
	authModeFlag := tinyflags.Enum(flags, "auth-mode", auth.AuthModeNone, "Emergency override for the database-managed authentication mode",
		auth.AuthModeNone,
		auth.AuthModeLocal,
		auth.AuthModeTrustedProxy,
		auth.AuthModeOIDC,
	).
		Placeholder("MODE")

	// Trusted-proxy
	flags.StringSliceVar(&cfg.TrustedUsernameHeaders, "trusted-username-headers", trustedUsernameHeaders, "Trusted-proxy username headers used only with the authentication override").
		Value()
	flags.StringSliceVar(&cfg.TrustedEmailHeaders, "trusted-email-headers", trustedEmailHeaders, "Trusted-proxy email headers used only with the authentication override").
		Value()
	flags.StringSliceVar(&cfg.TrustedDisplayNameHeaders, "trusted-display-name-headers", trustedDisplayNameHeaders, "Trusted-proxy display-name headers used only with the authentication override").
		Value()

	// OIDC
	flags.StringVar(&cfg.OIDCIssuer, "oidc-issuer", "", "OIDC issuer used only with the authentication override").
		Placeholder("URL").
		Value()
	flags.StringVar(&cfg.OIDCClientID, "oidc-client-id", "", "OIDC client ID used only with the authentication override").
		Value()
	flags.StringVar(&cfg.OIDCClientSecret, "oidc-client-secret", "", "OIDC client secret used when OIDC is enabled in the administration UI").
		OverriddenValueMaskFn(tinyflags.MaskFirstLast).
		Value()
	flags.StringVar(&cfg.OIDCSessionSecret, "oidc-session-secret", "", "Secret used to sign OIDC login state and session cookies").
		OverriddenValueMaskFn(tinyflags.MaskFirstLast).
		Validate(func(s string) error {
			if s == "" {
				return nil
			}
			if len(s) < 32 {
				return errors.New("oidc session secret must be at least 32 characters")
			}

			return nil
		}).
		Value()
	flags.StringVar(&cfg.EncryptionKey, "encryption-key", "", "Base64-encoded 32-byte key used to encrypt sensitive application settings").
		OverriddenValueMaskFn(tinyflags.MaskFirstLast).
		Validate(secrets.ValidateKey).
		Value()

	// Logging
	logFormat := tinyflags.Enum(flags, "log-format", logging.LogFormatJSON, "Log output format", logging.LogFormatText, logging.LogFormatJSON).
		Short("l").
		Placeholder("FORMAT").
		Value()
	flags.BoolVar(&cfg.Debug, "debug", false, "Enable verbose diagnostic logging").
		Short("d").
		Value()
	flags.BoolVar(&cfg.DebugRenderTimings, "debug-render-timings", false, "Log detailed page handler, Markdown, and WASM timings").
		Value()
	flags.BoolVar(&cfg.AccessLog, "access-log", false, "Enable HTTP request access logging").
		Value()

	return func() Config {
		resolved := cfg

		if authModeFlag.Changed() {
			resolved.AuthModeOverride = *authModeFlag.Value()
		}

		resolved.ListenAddress = (*listen).String()
		resolved.LogFormat = *logFormat

		return resolved
	}
}
