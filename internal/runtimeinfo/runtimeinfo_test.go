package runtimeinfo

import (
	"testing"
	"time"

	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedConfigurationUsesExactOrigins(t *testing.T) {
	t.Parallel()

	cfg := flags.Config{
		ListenAddress: "127.0.0.1:9090",
		DatabaseURL:   "postgres://example/kumbuka",
		OverrideOrigins: map[string]tinyflags.ValueOrigin{
			"listen-address": {Source: tinyflags.ValueSourceFlag, Key: "-a"},
			"database-url":   {Source: tinyflags.ValueSourceEnvironment, Key: "CUSTOM_DATABASE_DSN"},
		},
	}

	info := New(cfg, false)
	server := info.ManagedConfiguration[0]

	assert.Equal(t, "Flag · -a", server.Items[0].Source)
	assert.Equal(t, "Environment · CUSTOM_DATABASE_DSN", server.Items[1].Source)
	assert.Equal(t, "Default", server.Items[2].Source)
}

func TestNewRegistrationOverride(t *testing.T) {
	t.Parallel()

	t.Run("unset", func(t *testing.T) {
		t.Parallel()

		info := New(flags.Config{}, false)

		assert.False(t, info.UserRegistrationOverrideConfigured)
		assert.False(t, info.AllowUserRegistrationOverride)
	})

	t.Run("enabled", func(t *testing.T) {
		t.Parallel()

		enabled := true
		info := New(flags.Config{AllowUserRegistrationOverride: &enabled}, false)

		assert.True(t, info.UserRegistrationOverrideConfigured)
		assert.True(t, info.AllowUserRegistrationOverride)
	})

	t.Run("disabled", func(t *testing.T) {
		t.Parallel()

		enabled := false
		info := New(flags.Config{AllowUserRegistrationOverride: &enabled}, false)

		assert.True(t, info.UserRegistrationOverrideConfigured)
		assert.False(t, info.AllowUserRegistrationOverride)
	})
}

func TestManagedConfigurationReportsDatabaseMaxConns(t *testing.T) {
	t.Parallel()

	cfg := flags.Config{
		DatabaseMaxConns: 36,
		OverrideOrigins: map[string]tinyflags.ValueOrigin{
			"database-max-conns": {Source: tinyflags.ValueSourceEnvironment, Key: "KUMBUKA__DATABASE_MAX_CONNS"},
		},
	}
	info := New(cfg, false)

	for _, item := range info.ManagedConfiguration[0].Items {
		if item.Name != "Database max connections" {
			continue
		}

		assert.Equal(t, "36", item.Value)
		assert.Equal(t, "Environment · KUMBUKA__DATABASE_MAX_CONNS", item.Source)
		return
	}

	assert.Fail(t, "Database max connections configuration item not found")
}

func TestManagedConfigurationReportsDatabaseMinIdleConns(t *testing.T) {
	t.Parallel()

	cfg := flags.Config{
		DatabaseMinIdleConns: 4,
		OverrideOrigins: map[string]tinyflags.ValueOrigin{
			"database-min-idle-conns": {Source: tinyflags.ValueSourceFlag, Key: "--database-min-idle-conns"},
		},
	}
	info := New(cfg, false)

	for _, item := range info.ManagedConfiguration[0].Items {
		if item.Name != "Database minimum idle connections" {
			continue
		}

		assert.Equal(t, "4", item.Value)
		assert.Equal(t, "Flag · --database-min-idle-conns", item.Source)
		return
	}

	assert.Fail(t, "Database minimum idle connections configuration item not found")
}

func TestNewRedactsManagedSecrets(t *testing.T) {
	t.Parallel()

	cfg := flags.Config{
		DatabaseURL:               "postgres://secret@example/kumbuka",
		OIDCClientSecret:          "client-secret",
		OIDCSessionSecret:         "01234567890123456789012345678901",
		PluginUpdateCheckInterval: 15 * time.Minute,
	}

	info := New(cfg, true)
	require.Len(t, info.ManagedConfiguration, 3)
	assert.Equal(t, "15m", info.PluginUpdateCheckInterval)
	assert.True(t, info.EncryptionKeyConfigured)

	server := info.ManagedConfiguration[0]
	require.GreaterOrEqual(t, len(server.Items), 2)
	assert.Equal(t, "Database URL", server.Items[1].Name)
	assert.Equal(t, "Configured", server.Items[1].Value)
	assert.NotContains(t, server.Items[1].Value, "secret")

	authentication := info.ManagedConfiguration[1]
	require.Len(t, authentication.Items, 13)
	assert.Equal(t, "Configured", authentication.Items[10].Value)
	assert.Equal(t, "Configured", authentication.Items[11].Value)
	assert.Equal(t, "Configured", authentication.Items[12].Value)
	assert.NotContains(t, authentication.Items[10].Value, "client-secret")
	assert.NotContains(t, authentication.Items[11].Value, cfg.OIDCSessionSecret)
}

// TestManagedConfigurationReportsDisabledMetrics verifies administrators can see the effective deployment setting.
func TestManagedConfigurationReportsDisabledMetrics(t *testing.T) {
	t.Parallel()

	cfg := flags.Config{
		DisableMetrics:  true,
		OverrideOrigins: map[string]tinyflags.ValueOrigin{"disable-metrics": {Source: tinyflags.ValueSourceFlag, Key: "--disable-metrics"}},
	}
	info := New(cfg, false)

	server := info.ManagedConfiguration[0]
	for _, item := range server.Items {
		if item.Name != "Prometheus metrics" {
			continue
		}

		assert.Equal(t, "Disabled", item.Value)
		assert.Equal(t, "Flag · --disable-metrics", item.Source)
		return
	}

	assert.Fail(t, "Prometheus metrics configuration item not found")
}
