// Package flags parses deployment-level configuration for the Kumbuka server.
package cli

import (
	"time"

	appkitauth "github.com/containeroo/appkit/auth"

	appkitconfig "github.com/containeroo/appkit/config"
	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/pdf"
)

// DefaultPluginUpdateCheckInterval is the default background plugin catalog refresh interval.
const DefaultPluginUpdateCheckInterval = 60 * time.Minute

// sectionServices groups deployment-level overrides for external services.
const sectionServices = "Services"

// Flags contains deployment-level runtime configuration for Kumbuka.
type Flags struct {
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
func Parse(args []string, version string) (flags Flags, err error) {
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

	finalizeServer := appkitconfig.RegisterServer(tf, &flags.Server, appkitconfig.ServerOptions{})
	appkitconfig.RegisterPostgres(tf, &flags.Database, appkitconfig.PostgresOptions{})
	finalizeAuthentication := appkitconfig.RegisterAuthentication(
		tf,
		&flags.Authentication,
		appkitconfig.AuthenticationOptions{},
	)
	appkitconfig.RegisterProxy(tf, &flags.Proxy)
	appkitconfig.RegisterOIDC(tf, &flags.OIDC)
	finalizeLogging := appkitconfig.RegisterLogging(tf, &flags.Logging)
	appkitconfig.RegisterObservability(tf, &flags.Observability)

	registerApplicationFlags(tf, &flags)
	registerServiceFlags(tf, &flags)

	if err := tf.Parse(args); err != nil {
		return Flags{}, err
	}

	finalizeServer()
	finalizeAuthentication()
	finalizeLogging()
	flags.Overrides = tf.Overrides()

	return flags, nil
}

// AuthenticationModeOverride returns the configured authentication mode or the zero value when database settings apply.
func (c Flags) AuthenticationModeOverride() appkitauth.Mode {
	if c.Authentication.ModeOverride == nil {
		return ""
	}
	return *c.Authentication.ModeOverride
}

// registerApplicationFlags registers Kumbuka-specific server behavior.
func registerApplicationFlags(tf *tinyflags.FlagSet, cfg *Flags) {
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
func registerServiceFlags(tf *tinyflags.FlagSet, cfg *Flags) {
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
