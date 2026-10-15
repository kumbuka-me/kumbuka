// Package flags parses deployment-level configuration for the Kumbuka server.
package flags

import (
	"time"

	appkitconfig "github.com/containeroo/appkit/config"
	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/pdf"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// DefaultPluginUpdateCheckInterval is the default background plugin catalog refresh interval.
const DefaultPluginUpdateCheckInterval = 60 * time.Minute

const (
	sectionServices = "Services"
)

// Config contains deployment-level runtime configuration for Kumbuka.
type Config struct {
	// Server contains shared HTTP listener configuration.
	Server appkitconfig.Server
	// Database contains shared PostgreSQL configuration.
	Database appkitconfig.Postgres
	// Authentication contains shared authentication configuration.
	Authentication appkitconfig.Authentication
	// Proxy contains shared trusted-proxy configuration.
	Proxy appkitconfig.Proxy
	// OIDC contains shared OpenID Connect configuration.
	OIDC appkitconfig.OIDC
	// Logging contains shared logging configuration.
	Logging appkitconfig.Logging
	// Observability contains shared metrics and diagnostics configuration.
	Observability appkitconfig.Observability
	// PDFURL optionally overrides the persisted PDF rendering endpoint for this process.
	PDFURL string
	// PluginUpdateCheckInterval controls scheduled first-party catalog checks; zero keeps only manual checks.
	PluginUpdateCheckInterval time.Duration
	// ReadOnly blocks state-changing application requests while allowing authentication flows.
	ReadOnly bool
	// ThemeDirectory optionally overlays embedded themes with TOML files from disk.
	ThemeDirectory string
	// Overrides records configuration values explicitly overridden by flags or environment variables.
	Overrides tinyflags.Overrides
}

// Parse parses command-line arguments into application configuration.
func Parse(args []string, version string) (Config, error) {
	cfg := Config{}

	tf := appkitconfig.NewFlagSet("kumbuka", "KUMBUKA", version)
	tf.SectionOrder(
		appkitconfig.SectionServer,
		appkitconfig.SectionDatabase,
		sectionServices,
		appkitconfig.SectionAuthentication,
		appkitconfig.SectionProxy,
		appkitconfig.SectionOIDC,
		appkitconfig.SectionLogging,
		tinyflags.UnnamedSection,
	)

	finalizeServer := appkitconfig.RegisterServer(tf, &cfg.Server, appkitconfig.ServerOptions{})
	appkitconfig.RegisterPostgres(tf, &cfg.Database, appkitconfig.PostgresOptions{})
	finalizeAuthentication := appkitconfig.RegisterAuthentication(
		tf,
		&cfg.Authentication,
		appkitconfig.AuthenticationOptions{},
	)
	appkitconfig.RegisterProxy(tf, &cfg.Proxy)
	appkitconfig.RegisterOIDC(tf, &cfg.OIDC)
	finalizeLogging := appkitconfig.RegisterLogging(tf, &cfg.Logging)
	appkitconfig.RegisterObservability(tf, &cfg.Observability)
	registerApplicationFlags(tf, &cfg)
	registerServiceFlags(tf, &cfg)

	if err := tf.Parse(args); err != nil {
		return Config{}, err
	}

	finalizeServer()
	finalizeAuthentication()
	finalizeLogging()
	cfg.Overrides = tf.Overrides()

	return cfg, nil
}

// AuthenticationModeOverride returns Kumbuka's domain value for the Appkit mode override.
func (c Config) AuthenticationModeOverride() domain.AuthMode {
	if c.Authentication.ModeOverride == nil {
		return ""
	}
	return domain.AuthMode(*c.Authentication.ModeOverride)
}

// registerApplicationFlags registers Kumbuka-specific server behavior.
func registerApplicationFlags(tf *tinyflags.FlagSet, cfg *Config) {
	tf.BoolVar(
		&cfg.ReadOnly,
		"read-only",
		false,
		"Block state-changing application requests while keeping reads and authentication available",
	).
		Section(appkitconfig.SectionServer).
		Value()

	tf.StringVar(
		&cfg.ThemeDirectory,
		"theme-directory",
		"",
		"Directory containing custom theme TOML files that override or extend embedded themes",
	).
		Section(appkitconfig.SectionServer).
		Placeholder("DIR").
		Value()
}

// registerServiceFlags registers deployment overrides for external application services.
func registerServiceFlags(tf *tinyflags.FlagSet, cfg *Config) {
	tf.StringVar(
		&cfg.PDFURL,
		"pdf-url",
		"",
		"Deployment override for the PDF service POST URL, including its path",
	).
		Section(sectionServices).
		Placeholder("URL").
		Validate(pdf.ValidateURL).
		Value()

	tf.DurationVar(
		&cfg.PluginUpdateCheckInterval,
		"plugin-update-check-interval",
		DefaultPluginUpdateCheckInterval,
		"How often Kumbuka checks the first-party plugin update catalog; set to 0 to disable scheduled checks",
	).
		Section(sectionServices).
		Validate(tinyflags.NonNegative[time.Duration]()).
		Value()
}
