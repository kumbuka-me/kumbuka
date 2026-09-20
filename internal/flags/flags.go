// Package flags parses deployment-level configuration for the Kumbuka server.
package flags

import (
	"errors"
	"net"
	"strings"
	"time"

	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/pdf"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/logging"
)

// DefaultPluginUpdateCheckInterval is the default background plugin catalog refresh interval.
const DefaultPluginUpdateCheckInterval = 15 * time.Minute

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

var trustedGroupHeaders = []string{
	"X-Forwarded-Groups",
	"X-Auth-Request-Groups",
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
	// PluginUpdateCheckInterval controls scheduled first-party catalog checks; zero keeps only manual checks.
	PluginUpdateCheckInterval time.Duration
	// AllowUserRegistrationOverride overrides the persisted registration setting when non-nil.
	AllowUserRegistrationOverride *bool
	// ReadOnly blocks state-changing application requests while allowing authentication flows.
	ReadOnly bool
	// AuthModeOverride forces one browser authentication mode for recovery when non-empty.
	AuthModeOverride domain.AuthMode
	// TrustedUsernameHeaders are used only by the trusted-proxy runtime override.
	TrustedUsernameHeaders []string
	// TrustedEmailHeaders are used only by the trusted-proxy runtime override.
	TrustedEmailHeaders []string
	// TrustedDisplayNameHeaders are used only by the trusted-proxy runtime override.
	TrustedDisplayNameHeaders []string
	// TrustedGroupHeaders are used only by the trusted-proxy runtime override.
	TrustedGroupHeaders []string
	// TrustedAdminGroup grants administrator access when asserted by the trusted-proxy runtime override.
	TrustedAdminGroup string
	// OIDCIssuer is used only by the OIDC runtime override.
	OIDCIssuer string
	// OIDCClientID is used only by the OIDC runtime override.
	OIDCClientID string
	// OIDCGroupClaim names the group-membership claim used only by the OIDC runtime override.
	OIDCGroupClaim string
	// OIDCAdminGroup grants administrator access when asserted by the OIDC runtime override.
	OIDCAdminGroup string
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
	// Overrides records configuration values explicitly overridden by flags or environment variables.
	Overrides map[string]any
	// OverrideSources records whether each explicit override came from a flag or environment variable.
	OverrideSources map[string]string
}

// Parse parses command-line arguments into application configuration.
func Parse(args []string, version string) (Config, error) {
	cfg := Config{}

	tf := tinyflags.NewFlagSet("kumbuka", tinyflags.ContinueOnError)
	tf.EnvPrefix("KUMBUKA_")
	tf.Version(version)

	// Server
	listen := tf.TCPAddr("listen-address", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}, "Address on which the web server listens").
		Short("a").
		Placeholder("ADDR").
		Value()

	tf.StringVar(&cfg.DatabaseURL, "database-url", "", "PostgreSQL connection URL").
		Required().
		Placeholder("URL").
		OverriddenValueMaskFn(tinyflags.MaskPostgresURL).
		Value()
	tf.StringVar(&cfg.PublicURL, "public-url", "http://localhost:8080", "Externally visible base URL").
		Placeholder("URL").
		Value()
	tf.StringVar(&cfg.PDFURL, "pdf-url", "", "Deployment override for the PDF service POST URL, including its path").
		Placeholder("URL").
		Validate(pdf.ValidateURL).
		Value()
	tf.DurationVar(
		&cfg.PluginUpdateCheckInterval,
		"plugin-update-check-interval",
		DefaultPluginUpdateCheckInterval,
		"How often Kumbuka checks the first-party plugin update catalog; set to 0 to disable scheduled checks",
	).
		Validate(func(interval time.Duration) error {
			if interval < 0 {
				return errors.New("plugin update check interval must not be negative")
			}

			return nil
		}).
		Value()
	allowUserRegistrationFlag := tf.BoolVar(
		ToPtr(false),
		"allow-user-registration",
		false,
		"Deployment override for whether unknown OIDC or trusted-proxy identities may create accounts",
	).Strict()
	tf.BoolVar(&cfg.ReadOnly, "read-only", false, "Block state-changing application requests while keeping reads and authentication available").
		Value()
	tf.BoolVar(&cfg.LocalLogin, "local-login", false, "Enable the local recovery login alongside the configured authentication mode").
		Value()
	tf.StringVar(&cfg.ThemeDirectory, "theme-directory", "", "Directory containing custom theme TOML files that override or extend embedded themes").
		Placeholder("DIR").
		Value()

	// Auth
	authModeFlag := tinyflags.Enum(tf, "auth-mode", domain.AuthModeNone, "Emergency override for the database-managed authentication mode",
		domain.AuthModeNone,
		domain.AuthModeLocal,
		domain.AuthModeTrustedProxy,
		domain.AuthModeOIDC,
	).
		Placeholder("MODE")

	// Trusted-proxy
	tf.StringSliceVar(&cfg.TrustedUsernameHeaders, "trusted-username-headers", trustedUsernameHeaders, "Trusted-proxy username headers used only with the authentication override").
		Value()
	tf.StringSliceVar(&cfg.TrustedEmailHeaders, "trusted-email-headers", trustedEmailHeaders, "Trusted-proxy email headers used only with the authentication override").
		Value()
	tf.StringSliceVar(&cfg.TrustedDisplayNameHeaders, "trusted-display-name-headers", trustedDisplayNameHeaders, "Trusted-proxy display-name headers used only with the authentication override").
		Value()
	tf.StringSliceVar(&cfg.TrustedGroupHeaders, "trusted-group-headers", trustedGroupHeaders, "Trusted-proxy group headers used only with the authentication override").
		Value()
	tf.StringVar(&cfg.TrustedAdminGroup, "trusted-admin-group", "", "Trusted-proxy group that grants administrator access with the authentication override").
		Value()

	// OIDC
	tf.StringVar(&cfg.OIDCIssuer, "oidc-issuer", "", "OIDC issuer used only with the authentication override").
		Placeholder("URL").
		Value()
	tf.StringVar(&cfg.OIDCClientID, "oidc-client-id", "", "OIDC client ID used only with the authentication override").
		Value()
	tf.StringVar(&cfg.OIDCGroupClaim, "oidc-group-claim", "groups", "OIDC group-membership claim used only with the authentication override").
		Value()
	tf.StringVar(&cfg.OIDCAdminGroup, "oidc-admin-group", "", "OIDC group that grants administrator access with the authentication override").
		Value()
	tf.StringVar(&cfg.OIDCClientSecret, "oidc-client-secret", "", "OIDC client secret used when OIDC is enabled in the administration UI").
		OverriddenValueMaskFn(tinyflags.MaskFirstLast).
		Value()
	tf.StringVar(&cfg.OIDCSessionSecret, "oidc-session-secret", "", "Secret used to sign OIDC login state and session cookies").
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
	tf.StringVar(&cfg.EncryptionKey, "encryption-key", "", "Base64-encoded 32-byte key used to encrypt sensitive application settings").
		OverriddenValueMaskFn(tinyflags.MaskFirstLast).
		Validate(secrets.ValidateKey).
		Value()

	// Logging
	logFormat := tinyflags.Enum(tf, "log-format", logging.LogFormatJSON, "Log output format", logging.LogFormatText, logging.LogFormatJSON).
		Short("l").
		Placeholder("FORMAT").
		Value()
	tf.BoolVar(&cfg.Debug, "debug", false, "Enable verbose diagnostic logging").
		Short("d").
		Value()
	tf.BoolVar(&cfg.DebugRenderTimings, "debug-render-timings", false, "Log detailed page handler, Markdown, and WASM timings").
		Value()
	tf.BoolVar(&cfg.AccessLog, "access-log", false, "Enable HTTP request access logging").
		Value()

	if err := tf.Parse(args); err != nil {
		return Config{}, err
	}

	if authModeFlag.Changed() {
		cfg.AuthModeOverride = *authModeFlag.Value()
	}
	if allowUserRegistrationFlag.Changed() {
		cfg.AllowUserRegistrationOverride = allowUserRegistrationFlag.Value()
	}

	cfg.ListenAddress = (*listen).String()
	cfg.LogFormat = *logFormat
	cfg.Overrides = tf.OverriddenValues()
	cfg.OverrideSources = overrideSources(args, cfg.Overrides)

	return cfg, nil
}

// overrideSources identifies whether each explicit deployment override came from a flag or environment variable.
func overrideSources(args []string, overrides map[string]any) map[string]string {
	sources := make(map[string]string, len(overrides))
	for name := range overrides {
		sources[name] = "Environment"
		if argumentSetsFlag(args, name) {
			sources[name] = "Flag"
		}
	}

	return sources
}

// argumentSetsFlag reports whether command-line arguments explicitly set the named long flag.
func argumentSetsFlag(args []string, name string) bool {
	long := "--" + name
	short := map[string]string{
		"listen-address": "-a",
		"log-format":     "-l",
		"debug":          "-d",
	}[name]

	for _, argument := range args {
		if argument == long || strings.HasPrefix(argument, long+"=") {
			return true
		}
		if short != "" && (argument == short || strings.HasPrefix(argument, short+"=")) {
			return true
		}
	}

	return false
}
