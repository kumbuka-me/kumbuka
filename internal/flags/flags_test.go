package flags

import (
	"testing"
	"time"

	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseTestConfig parses one isolated flag test configuration.
func parseTestConfig(args []string) (Config, error) {
	return Parse(args, "test")
}

// TestEnvironmentPrefix verifies the corresponding flag configuration behavior.
func TestEnvironmentPrefix(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__AUTH_MODE", string(domain.AuthModeTrustedProxy))

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, "postgres://example/kumbuka", cfg.DatabaseURL)
	assert.Equal(t, domain.AuthModeTrustedProxy, cfg.AuthModeOverride)
}

// TestOIDCSessionSecretAndEncryptionKeyFromEnvironment verifies the corresponding flag configuration behavior.
func TestOIDCSessionSecretAndEncryptionKeyFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__OIDC_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("KUMBUKA__ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", cfg.OIDCSessionSecret)
	assert.Equal(t, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", cfg.EncryptionKey)
}

// TestAuthModeRejectsUnknownValue verifies the corresponding flag configuration behavior.
func TestAuthModeRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	_, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--auth-mode", "invalid",
	})

	require.Error(t, err)
}

// TestAuthModeDefaultsToDatabaseManaged verifies the corresponding flag configuration behavior.
func TestAuthModeDefaultsToDatabaseManaged(t *testing.T) {
	t.Setenv("KUMBUKA__AUTH_MODE", "")

	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

	require.NoError(t, err)
	assert.Empty(t, cfg.AuthModeOverride)
}

// TestLocalRecoveryLoginCanBeEnabledFromEnvironment verifies the corresponding flag configuration behavior.
func TestLocalRecoveryLoginCanBeEnabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__LOCAL_LOGIN", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.True(t, cfg.LocalLogin)
}

// TestUserRegistrationDefaultsToDatabaseManaged verifies the corresponding flag configuration behavior.
func TestUserRegistrationDefaultsToDatabaseManaged(t *testing.T) {
	t.Setenv("KUMBUKA__ALLOW_USER_REGISTRATION", "")

	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

	require.NoError(t, err)
	assert.Nil(t, cfg.AllowUserRegistrationOverride)
}

// TestUserRegistrationCanBeEnabledFromEnvironment verifies the corresponding flag configuration behavior.
func TestUserRegistrationCanBeEnabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__ALLOW_USER_REGISTRATION", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	require.NotNil(t, cfg.AllowUserRegistrationOverride)
	assert.True(t, *cfg.AllowUserRegistrationOverride)
}

// TestUserRegistrationCanBeDisabledByFlag verifies the corresponding flag configuration behavior.
func TestUserRegistrationCanBeDisabledByFlag(t *testing.T) {
	t.Parallel()

	cfg, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--allow-user-registration=false",
	})

	require.NoError(t, err)
	require.NotNil(t, cfg.AllowUserRegistrationOverride)
	assert.False(t, *cfg.AllowUserRegistrationOverride)
}

func TestPluginUpdateCheckIntervalDefaultsToOneHour(t *testing.T) {
	t.Setenv("KUMBUKA__PLUGIN_UPDATE_CHECK_INTERVAL", "")

	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

	require.NoError(t, err)
	assert.Equal(t, 1*time.Hour, cfg.PluginUpdateCheckInterval)
}

// TestPluginUpdateCheckIntervalCanBeConfiguredFromEnvironment verifies the corresponding flag configuration behavior.
func TestPluginUpdateCheckIntervalCanBeConfiguredFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__PLUGIN_UPDATE_CHECK_INTERVAL", "1h30m")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, 90*time.Minute, cfg.PluginUpdateCheckInterval)
}

// TestPluginUpdateCheckIntervalCanDisableScheduledChecks verifies the corresponding flag configuration behavior.
func TestPluginUpdateCheckIntervalCanDisableScheduledChecks(t *testing.T) {
	t.Parallel()

	cfg, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--plugin-update-check-interval", "0",
	})

	require.NoError(t, err)
	assert.Zero(t, cfg.PluginUpdateCheckInterval)
}

// TestPluginUpdateCheckIntervalRejectsNegativeDuration verifies the corresponding flag configuration behavior.
func TestPluginUpdateCheckIntervalRejectsNegativeDuration(t *testing.T) {
	t.Parallel()

	_, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--plugin-update-check-interval", "-1m",
	})

	require.Error(t, err)
}

// TestRenderTimingsCanBeEnabledFromEnvironment verifies the corresponding flag configuration behavior.
func TestRenderTimingsCanBeEnabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__DEBUG_RENDER_TIMINGS", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.True(t, cfg.DebugRenderTimings)
}

// TestOIDCSecretsRemainDeploymentConfiguration verifies the corresponding flag configuration behavior.
func TestOIDCSecretsRemainDeploymentConfiguration(t *testing.T) {
	t.Parallel()

	cfg, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--oidc-client-secret", "client-secret",
		"--oidc-session-secret", "0123456789abcdef0123456789abcdef",
	})

	require.NoError(t, err)
	assert.Equal(t, "client-secret", cfg.OIDCClientSecret)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", cfg.OIDCSessionSecret)
}

// TestOverriddenValuesMaskSecrets verifies the corresponding flag configuration behavior.
func TestOverriddenValuesMaskSecrets(t *testing.T) {
	t.Parallel()

	databaseURL := "postgres://kumbuka:database-secret@postgres:5432/kumbuka?sslmode=disable"
	oidcSecret := "oidc-client-secret-value"
	oidcSessionSecret := "0123456789abcdef0123456789abcdef"
	encryptionKey := "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

	cfg, err := parseTestConfig([]string{
		"--database-url", databaseURL,
		"--oidc-client-secret", oidcSecret,
		"--oidc-session-secret", oidcSessionSecret,
		"--encryption-key", encryptionKey,
	})
	require.NoError(t, err)

	overrides := cfg.Overrides

	assert.Contains(t, overrides, "database-url")
	assert.Contains(t, overrides, "oidc-client-secret")
	assert.Contains(t, overrides, "oidc-session-secret")
	assert.Contains(t, overrides, "encryption-key")
	assert.NotEqual(t, databaseURL, overrides["database-url"])
	assert.NotEqual(t, oidcSecret, overrides["oidc-client-secret"])
	assert.NotEqual(t, oidcSessionSecret, overrides["oidc-session-secret"])
	assert.NotEqual(t, encryptionKey, overrides["encryption-key"])
}

// TestOverrideOrigins identifies the exact flag or environment input behind each deployment override.
func TestOverrideOrigins(t *testing.T) {
	t.Run("flags", func(t *testing.T) {
		cfg, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--public-url", "https://kumbuka.example.test",
			"-a", "127.0.0.1:9090",
		})

		require.NoError(t, err)
		assert.Equal(t, tinyflags.ValueOrigin{Source: tinyflags.ValueSourceFlag, Key: "--database-url"}, cfg.OverrideOrigins["database-url"])
		assert.Equal(t, tinyflags.ValueOrigin{Source: tinyflags.ValueSourceFlag, Key: "--public-url"}, cfg.OverrideOrigins["public-url"])
		assert.Equal(t, tinyflags.ValueOrigin{Source: tinyflags.ValueSourceFlag, Key: "-a"}, cfg.OverrideOrigins["listen-address"])
	})

	t.Run("environment", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__PUBLIC_URL", "https://kumbuka.example.test")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.Equal(t, tinyflags.ValueOrigin{Source: tinyflags.ValueSourceEnvironment, Key: "KUMBUKA__DATABASE_URL"}, cfg.OverrideOrigins["database-url"])
		assert.Equal(t, tinyflags.ValueOrigin{Source: tinyflags.ValueSourceEnvironment, Key: "KUMBUKA__PUBLIC_URL"}, cfg.OverrideOrigins["public-url"])
	})
}

func TestDatabaseMaxConns(t *testing.T) {
	t.Run("default uses automatic sizing", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_MAX_CONNS", "")

		cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

		require.NoError(t, err)
		assert.Zero(t, cfg.DatabaseMaxConns)
	})

	t.Run("flag", func(t *testing.T) {
		cfg, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--database-max-conns", "36",
		})

		require.NoError(t, err)
		assert.Equal(t, int32(36), cfg.DatabaseMaxConns)
	})

	t.Run("environment", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__DATABASE_MAX_CONNS", "24")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.Equal(t, int32(24), cfg.DatabaseMaxConns)
	})

	t.Run("negative", func(t *testing.T) {
		_, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--database-max-conns", "-1",
		})

		require.Error(t, err)
	})

	t.Run("outside int32 range", func(t *testing.T) {
		_, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--database-max-conns", "2147483648",
		})

		require.Error(t, err)
	})
}

func TestDatabaseMinIdleConns(t *testing.T) {
	t.Run("default uses pgxpool default", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_MIN_IDLE_CONNS", "")

		cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

		require.NoError(t, err)
		assert.Zero(t, cfg.DatabaseMinIdleConns)
	})

	t.Run("flag", func(t *testing.T) {
		cfg, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--database-min-idle-conns", "4",
		})

		require.NoError(t, err)
		assert.Equal(t, int32(4), cfg.DatabaseMinIdleConns)
	})

	t.Run("environment", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__DATABASE_MIN_IDLE_CONNS", "3")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.Equal(t, int32(3), cfg.DatabaseMinIdleConns)
	})

	t.Run("negative", func(t *testing.T) {
		_, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--database-min-idle-conns", "-1",
		})

		require.Error(t, err)
	})

	t.Run("outside int32 range", func(t *testing.T) {
		_, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--database-min-idle-conns", "2147483648",
		})

		require.Error(t, err)
	})
}

// TestEncryptionKeyRejectsInvalidValue verifies the corresponding flag configuration behavior.
func TestEncryptionKeyRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	_, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--encryption-key", "not-a-32-byte-base64-key",
	})

	require.Error(t, err)
}

// TestPDFURLFromEnvironmentIncludesPath verifies the corresponding flag configuration behavior.
func TestPDFURLFromEnvironmentIncludesPath(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__PDF_URL", "http://html2pdf:8080/custom/render?profile=wiki")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, "http://html2pdf:8080/custom/render?profile=wiki", cfg.PDFURL)
}

// TestPDFURLValidation verifies the corresponding flag configuration behavior.
func TestPDFURLValidation(t *testing.T) {
	t.Run("missing HTTP scheme", func(t *testing.T) {
		_, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka", "--pdf-url", "pdf:8080/render"})
		require.Error(t, err)
	})

	t.Run("missing endpoint path", func(t *testing.T) {
		_, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka", "--pdf-url", "http://pdf:8080"})
		require.Error(t, err)
	})

	t.Run("file URL", func(t *testing.T) {
		_, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka", "--pdf-url", "file:///render"})
		require.Error(t, err)
	})

	t.Run("URL fragment", func(t *testing.T) {
		_, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka", "--pdf-url", "http://pdf/render#fragment"})
		require.Error(t, err)
	})

	t.Run("embedded credentials", func(t *testing.T) {
		_, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka", "--pdf-url", "http://user:password@pdf/render"})
		require.Error(t, err)
	})

	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

	require.NoError(t, err)
	assert.Empty(t, cfg.PDFURL)
}

// TestLocalAuthenticationOverride verifies the corresponding flag configuration behavior.
func TestLocalAuthenticationOverride(t *testing.T) {
	t.Parallel()
	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka", "--auth-mode", "local"})
	require.NoError(t, err)
	assert.Equal(t, domain.AuthModeLocal, cfg.AuthModeOverride)
}

// TestLocalAuthenticationOverrideFromEnvironment verifies the corresponding flag configuration behavior.
func TestLocalAuthenticationOverrideFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__AUTH_MODE", "local")
	cfg, err := parseTestConfig(nil)
	require.NoError(t, err)
	assert.Equal(t, domain.AuthModeLocal, cfg.AuthModeOverride)
}

// TestUserRegistrationFlagRequiresExplicitValue verifies the corresponding flag configuration behavior.
func TestUserRegistrationFlagRequiresExplicitValue(t *testing.T) {
	t.Parallel()

	_, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--allow-user-registration",
	})

	require.Error(t, err)
}

// TestReadOnlyCanBeEnabledFromEnvironment verifies the corresponding flag configuration behavior.
func TestReadOnlyCanBeEnabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__READ_ONLY", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.True(t, cfg.ReadOnly)
}

// TestMetricsAreEnabledByDefault verifies Prometheus metrics remain available unless explicitly disabled.
func TestMetricsAreEnabledByDefault(t *testing.T) {
	t.Setenv("KUMBUKA__DISABLE_METRICS", "")

	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

	require.NoError(t, err)
	assert.False(t, cfg.DisableMetrics)
}

// TestMetricsCanBeDisabledByFlag verifies the deployment can remove Prometheus metrics explicitly.
func TestMetricsCanBeDisabledByFlag(t *testing.T) {
	t.Parallel()

	cfg, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--disable-metrics",
	})

	require.NoError(t, err)
	assert.True(t, cfg.DisableMetrics)
}

// TestMetricsCanBeDisabledFromEnvironment verifies the disable switch follows normal deployment configuration.
func TestMetricsCanBeDisabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__DISABLE_METRICS", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.True(t, cfg.DisableMetrics)
}

// TestTrustedProxyAuthorizationOverridesFromEnvironment verifies the corresponding flag configuration behavior.
func TestTrustedProxyAuthorizationOverridesFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__TRUSTED_GROUP_HEADERS", "X-Groups,X-Teams")
	t.Setenv("KUMBUKA__TRUSTED_ADMIN_GROUP", "platform-admins")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"X-Groups", "X-Teams"}, cfg.TrustedGroupHeaders)
	assert.Equal(t, "platform-admins", cfg.TrustedAdminGroup)
}

// TestOIDCAuthorizationOverridesFromEnvironment verifies the corresponding flag configuration behavior.
func TestOIDCAuthorizationOverridesFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__OIDC_GROUP_CLAIM", "roles")
	t.Setenv("KUMBUKA__OIDC_ADMIN_GROUP", "wiki-admins")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, "roles", cfg.OIDCGroupClaim)
	assert.Equal(t, "wiki-admins", cfg.OIDCAdminGroup)
}
