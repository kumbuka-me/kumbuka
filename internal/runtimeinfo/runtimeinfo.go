// Package runtimeinfo maps deployment-owned process configuration to administrator-safe presentation data.
package runtimeinfo

import (
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// New returns administrator-safe runtime information for the active deployment. Parameters: - cfg: Parsed deployment-owned process configuration. - encryptionKeyConfigured: Whether application encryption is available without exposing its key. Returns: - webview.RuntimeInfo: Redacted runtime and managed-configuration presentation data.
func New(cfg flags.Config, encryptionKeyConfigured bool) webview.RuntimeInfo {
	registrationOverrideConfigured := cfg.AllowUserRegistrationOverride != nil
	allowUserRegistrationOverride := false
	if registrationOverrideConfigured {
		allowUserRegistrationOverride = *cfg.AllowUserRegistrationOverride
	}

	return webview.RuntimeInfo{
		ListenAddress:                      cfg.ListenAddress,
		PublicURL:                          cfg.PublicURL,
		PDFURL:                             cfg.PDFURL,
		ReadOnly:                           cfg.ReadOnly,
		UserRegistrationOverrideConfigured: registrationOverrideConfigured,
		AllowUserRegistrationOverride:      allowUserRegistrationOverride,
		AuthModeOverride:                   cfg.AuthModeOverride,
		OIDCIssuerOverride:                 cfg.OIDCIssuer,
		OIDCClientIDOverride:               cfg.OIDCClientID,
		TrustedUsernameHeadersOverride:     cfg.TrustedUsernameHeaders,
		TrustedEmailHeadersOverride:        cfg.TrustedEmailHeaders,
		TrustedDisplayNameHeadersOverride:  cfg.TrustedDisplayNameHeaders,
		TrustedGroupHeadersOverride:        cfg.TrustedGroupHeaders,
		TrustedAdminGroupOverride:          cfg.TrustedAdminGroup,
		OIDCGroupClaimOverride:             cfg.OIDCGroupClaim,
		OIDCAdminGroupOverride:             cfg.OIDCAdminGroup,
		OIDCClientSecretConfigured:         cfg.OIDCClientSecret != "",
		OIDCSessionSecretConfigured:        len(cfg.OIDCSessionSecret) >= 32,
		EncryptionKeyConfigured:            encryptionKeyConfigured,
		LocalLoginEnabled:                  cfg.LocalLogin,
		ThemeDirectory:                     cfg.ThemeDirectory,
		PluginUpdateCheckInterval:          pluginUpdateCheckIntervalLabel(cfg.PluginUpdateCheckInterval),
		ManagedConfiguration:               managedConfiguration(cfg, encryptionKeyConfigured),
	}
}

// managedConfiguration returns deployment-owned configuration using only administrator-safe values.
func managedConfiguration(cfg flags.Config, encryptionKeyConfigured bool) []webview.ManagedConfigurationGroup {
	return []webview.ManagedConfigurationGroup{
		{
			Name: "Server",
			Items: []webview.ManagedConfigurationItem{
				managedConfigurationItem(cfg, "Listen address", "listen-address", cfg.ListenAddress),
				managedConfigurationItem(cfg, "Database URL", "database-url", configuredLabel(cfg.DatabaseURL != "")),
				managedConfigurationItem(cfg, "Public URL", "public-url", cfg.PublicURL),
				managedConfigurationItem(cfg, "PDF URL override", "pdf-url", configuredValue(cfg.PDFURL)),
				managedConfigurationItem(cfg, "Plugin update checks", "plugin-update-check-interval", pluginUpdateCheckIntervalLabel(cfg.PluginUpdateCheckInterval)),
				managedConfigurationItem(cfg, "Allow new users override", "allow-user-registration", optionalBoolLabel(cfg.AllowUserRegistrationOverride)),
				managedConfigurationItem(cfg, "Read-only mode", "read-only", enabledLabel(cfg.ReadOnly)),
				managedConfigurationItem(cfg, "Prometheus metrics", "disable-metrics", enabledLabel(!cfg.DisableMetrics)),
				managedConfigurationItem(cfg, "Local recovery login", "local-login", enabledLabel(cfg.LocalLogin)),
				managedConfigurationItem(cfg, "Theme directory", "theme-directory", configuredValue(cfg.ThemeDirectory)),
			},
		},
		{
			Name: "Authentication and secrets",
			Items: []webview.ManagedConfigurationItem{
				managedConfigurationItem(cfg, "Authentication mode override", "auth-mode", configuredValue(string(cfg.AuthModeOverride))),
				managedConfigurationItem(cfg, "Trusted username headers", "trusted-username-headers", stringSliceLabel(cfg.TrustedUsernameHeaders)),
				managedConfigurationItem(cfg, "Trusted email headers", "trusted-email-headers", stringSliceLabel(cfg.TrustedEmailHeaders)),
				managedConfigurationItem(cfg, "Trusted display-name headers", "trusted-display-name-headers", stringSliceLabel(cfg.TrustedDisplayNameHeaders)),
				managedConfigurationItem(cfg, "Trusted group headers", "trusted-group-headers", stringSliceLabel(cfg.TrustedGroupHeaders)),
				managedConfigurationItem(cfg, "Trusted administrator group", "trusted-admin-group", configuredValue(cfg.TrustedAdminGroup)),
				managedConfigurationItem(cfg, "OIDC issuer override", "oidc-issuer", configuredValue(cfg.OIDCIssuer)),
				managedConfigurationItem(cfg, "OIDC client ID override", "oidc-client-id", configuredValue(cfg.OIDCClientID)),
				managedConfigurationItem(cfg, "OIDC group claim override", "oidc-group-claim", configuredValue(cfg.OIDCGroupClaim)),
				managedConfigurationItem(cfg, "OIDC administrator group override", "oidc-admin-group", configuredValue(cfg.OIDCAdminGroup)),
				managedConfigurationItem(cfg, "OIDC client secret", "oidc-client-secret", configuredLabel(cfg.OIDCClientSecret != "")),
				managedConfigurationItem(cfg, "OIDC session secret", "oidc-session-secret", configuredLabel(len(cfg.OIDCSessionSecret) >= 32)),
				managedConfigurationItem(cfg, "Application encryption key", "encryption-key", configuredLabel(encryptionKeyConfigured)),
			},
		},
		{
			Name: "Logging",
			Items: []webview.ManagedConfigurationItem{
				managedConfigurationItem(cfg, "Log format", "log-format", string(cfg.LogFormat)),
				managedConfigurationItem(cfg, "Debug logging", "debug", enabledLabel(cfg.Debug)),
				managedConfigurationItem(cfg, "Render timing diagnostics", "debug-render-timings", enabledLabel(cfg.DebugRenderTimings)),
				managedConfigurationItem(cfg, "Access log", "access-log", enabledLabel(cfg.AccessLog)),
			},
		},
	}
}

// managedConfigurationItem builds one safe deployment configuration row and source label.
func managedConfigurationItem(cfg flags.Config, name, flagName, value string) webview.ManagedConfigurationItem {
	return webview.ManagedConfigurationItem{
		Name:   name,
		Value:  value,
		Source: managedConfigurationSource(cfg, flagName),
	}
}

// managedConfigurationSource formats the source of one deployment-owned setting.
func managedConfigurationSource(cfg flags.Config, flagName string) string {
	source, overridden := cfg.OverrideSources[flagName]
	if !overridden {
		return "Default"
	}

	if source == "Flag" {
		return "Flag · --" + flagName
	}

	environment := "KUMBUKA__" + strings.ToUpper(strings.ReplaceAll(flagName, "-", "_"))
	return "Environment · " + environment
}

// configuredValue returns a readable label for an optional non-secret deployment value.
func configuredValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Not configured"
	}

	return value
}

// configuredLabel reports whether a secret or sensitive deployment value is available.
func configuredLabel(configured bool) string {
	if configured {
		return "Configured"
	}

	return "Not configured"
}

// enabledLabel reports a boolean deployment setting in administrator-facing language.
func enabledLabel(enabled bool) string {
	if enabled {
		return "Enabled"
	}

	return "Disabled"
}

// optionalBoolLabel reports an optional boolean deployment override.
func optionalBoolLabel(value *bool) string {
	if value == nil {
		return "Not configured"
	}

	return enabledLabel(*value)
}

// stringSliceLabel formats a deployment-owned string list without exposing hidden values.
func stringSliceLabel(values []string) string {
	if len(values) == 0 {
		return "Not configured"
	}

	return strings.Join(values, ", ")
}

// pluginUpdateCheckIntervalLabel formats the deployment plugin update interval for administrator display.
func pluginUpdateCheckIntervalLabel(interval time.Duration) string {
	if interval <= 0 {
		return "Disabled"
	}

	label := interval.String()
	if interval >= time.Minute && interval%time.Minute == 0 {
		return strings.TrimSuffix(label, "0s")
	}

	return label
}
