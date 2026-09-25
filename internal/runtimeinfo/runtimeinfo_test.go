package runtimeinfo

import (
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedConfigurationSourceDefaults(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "Default", managedConfigurationSource(flags.Config{}, "listen-address"))
}

func TestManagedConfigurationSourceReportsFlag(t *testing.T) {
	t.Parallel()

	cfg := flags.Config{OverrideSources: map[string]string{"listen-address": "Flag"}}
	assert.Equal(t, "Flag · --listen-address", managedConfigurationSource(cfg, "listen-address"))
}

func TestManagedConfigurationSourceReportsEnvironment(t *testing.T) {
	t.Parallel()

	cfg := flags.Config{OverrideSources: map[string]string{"database-url": "Environment"}}
	assert.Equal(t, "Environment · KUMBUKA__DATABASE_URL", managedConfigurationSource(cfg, "database-url"))
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
		OverrideSources: map[string]string{"disable-metrics": "Flag"},
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
