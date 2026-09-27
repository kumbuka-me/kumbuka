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

func TestParse(t *testing.T) {
	t.Run("environment prefix", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__AUTH_MODE", string(domain.AuthModeTrustedProxy))

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.Equal(t, "postgres://example/kumbuka", cfg.DatabaseURL)
		assert.Equal(t, domain.AuthModeTrustedProxy, cfg.AuthModeOverride)
	})

	t.Run("overridden values mask secrets", func(t *testing.T) {
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
	})

	t.Run("override origins", func(t *testing.T) {
		t.Run("flags", func(t *testing.T) {
			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--public-url", "https://kumbuka.example.test",
				"-a", "127.0.0.1:9090",
			})

			require.NoError(t, err)
			assert.Equal(
				t,
				tinyflags.ValueOrigin{
					Source: tinyflags.ValueSourceFlag,
					Key:    "--database-url",
				},
				cfg.OverrideOrigins["database-url"],
			)
			assert.Equal(
				t,
				tinyflags.ValueOrigin{
					Source: tinyflags.ValueSourceFlag,
					Key:    "--public-url",
				},
				cfg.OverrideOrigins["public-url"],
			)
			assert.Equal(
				t,
				tinyflags.ValueOrigin{
					Source: tinyflags.ValueSourceFlag,
					Key:    "-a",
				},
				cfg.OverrideOrigins["listen-address"],
			)
		})

		t.Run("environment", func(t *testing.T) {
			t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
			t.Setenv("KUMBUKA__PUBLIC_URL", "https://kumbuka.example.test")

			cfg, err := parseTestConfig(nil)

			require.NoError(t, err)
			assert.Equal(
				t,
				tinyflags.ValueOrigin{
					Source: tinyflags.ValueSourceEnvironment,
					Key:    "KUMBUKA__DATABASE_URL",
				},
				cfg.OverrideOrigins["database-url"],
			)
			assert.Equal(
				t,
				tinyflags.ValueOrigin{
					Source: tinyflags.ValueSourceEnvironment,
					Key:    "KUMBUKA__PUBLIC_URL",
				},
				cfg.OverrideOrigins["public-url"],
			)
		})
	})
}

func TestServerFlags(t *testing.T) {
	t.Run("database max connections", func(t *testing.T) {
		t.Run("default uses automatic sizing", func(t *testing.T) {
			t.Setenv("KUMBUKA__DATABASE_MAX_CONNS", "")

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
			})

			require.NoError(t, err)
			assert.Zero(t, cfg.DatabaseMaxConns)
		})

		t.Run("flag", func(t *testing.T) {
			t.Parallel()

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
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--database-max-conns", "-1",
			})

			require.Error(t, err)
		})

		t.Run("outside int32 range", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--database-max-conns", "2147483648",
			})

			require.Error(t, err)
		})
	})

	t.Run("database minimum idle connections", func(t *testing.T) {
		t.Run("default uses pgxpool default", func(t *testing.T) {
			t.Setenv("KUMBUKA__DATABASE_MIN_IDLE_CONNS", "")

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
			})

			require.NoError(t, err)
			assert.Zero(t, cfg.DatabaseMinIdleConns)
		})

		t.Run("flag", func(t *testing.T) {
			t.Parallel()

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
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--database-min-idle-conns", "-1",
			})

			require.Error(t, err)
		})

		t.Run("outside int32 range", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--database-min-idle-conns", "2147483648",
			})

			require.Error(t, err)
		})
	})

	t.Run("PDF URL", func(t *testing.T) {
		t.Run("environment includes path", func(t *testing.T) {
			t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
			t.Setenv(
				"KUMBUKA__PDF_URL",
				"http://html2pdf:8080/custom/render?profile=wiki",
			)

			cfg, err := parseTestConfig(nil)

			require.NoError(t, err)
			assert.Equal(
				t,
				"http://html2pdf:8080/custom/render?profile=wiki",
				cfg.PDFURL,
			)
		})

		t.Run("missing HTTP scheme", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--pdf-url", "pdf:8080/render",
			})

			require.Error(t, err)
		})

		t.Run("missing endpoint path", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--pdf-url", "http://pdf:8080",
			})

			require.Error(t, err)
		})

		t.Run("file URL", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--pdf-url", "file:///render",
			})

			require.Error(t, err)
		})

		t.Run("URL fragment", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--pdf-url", "http://pdf/render#fragment",
			})

			require.Error(t, err)
		})

		t.Run("embedded credentials", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--pdf-url", "http://user:password@pdf/render",
			})

			require.Error(t, err)
		})

		t.Run("empty by default", func(t *testing.T) {
			t.Setenv("KUMBUKA__PDF_URL", "")

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
			})

			require.NoError(t, err)
			assert.Empty(t, cfg.PDFURL)
		})
	})

	t.Run("plugin update check interval", func(t *testing.T) {
		t.Run("defaults to one hour", func(t *testing.T) {
			t.Setenv("KUMBUKA__PLUGIN_UPDATE_CHECK_INTERVAL", "")

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
			})

			require.NoError(t, err)
			assert.Equal(t, time.Hour, cfg.PluginUpdateCheckInterval)
		})

		t.Run("environment", func(t *testing.T) {
			t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
			t.Setenv("KUMBUKA__PLUGIN_UPDATE_CHECK_INTERVAL", "1h30m")

			cfg, err := parseTestConfig(nil)

			require.NoError(t, err)
			assert.Equal(t, 90*time.Minute, cfg.PluginUpdateCheckInterval)
		})

		t.Run("zero disables scheduled checks", func(t *testing.T) {
			t.Parallel()

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--plugin-update-check-interval", "0",
			})

			require.NoError(t, err)
			assert.Zero(t, cfg.PluginUpdateCheckInterval)
		})

		t.Run("rejects negative duration", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--plugin-update-check-interval", "-1m",
			})

			require.Error(t, err)
		})
	})

	t.Run("user registration override", func(t *testing.T) {
		t.Run("defaults to database managed", func(t *testing.T) {
			t.Setenv("KUMBUKA__ALLOW_USER_REGISTRATION", "")

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
			})

			require.NoError(t, err)
			assert.Nil(t, cfg.AllowUserRegistrationOverride)
		})

		t.Run("environment can enable", func(t *testing.T) {
			t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
			t.Setenv("KUMBUKA__ALLOW_USER_REGISTRATION", "true")

			cfg, err := parseTestConfig(nil)

			require.NoError(t, err)
			require.NotNil(t, cfg.AllowUserRegistrationOverride)
			assert.True(t, *cfg.AllowUserRegistrationOverride)
		})

		t.Run("flag can disable", func(t *testing.T) {
			t.Parallel()

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--allow-user-registration=false",
			})

			require.NoError(t, err)
			require.NotNil(t, cfg.AllowUserRegistrationOverride)
			assert.False(t, *cfg.AllowUserRegistrationOverride)
		})

		t.Run("flag requires explicit value", func(t *testing.T) {
			t.Parallel()

			_, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--allow-user-registration",
			})

			require.Error(t, err)
		})
	})

	t.Run("read only", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__READ_ONLY", "true")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.True(t, cfg.ReadOnly)
	})

	t.Run("metrics", func(t *testing.T) {
		t.Run("enabled by default", func(t *testing.T) {
			t.Setenv("KUMBUKA__DISABLE_METRICS", "")

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
			})

			require.NoError(t, err)
			assert.False(t, cfg.DisableMetrics)
		})

		t.Run("disabled by flag", func(t *testing.T) {
			t.Parallel()

			cfg, err := parseTestConfig([]string{
				"--database-url", "postgres://example/kumbuka",
				"--disable-metrics",
			})

			require.NoError(t, err)
			assert.True(t, cfg.DisableMetrics)
		})

		t.Run("disabled by environment", func(t *testing.T) {
			t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
			t.Setenv("KUMBUKA__DISABLE_METRICS", "true")

			cfg, err := parseTestConfig(nil)

			require.NoError(t, err)
			assert.True(t, cfg.DisableMetrics)
		})
	})

	t.Run("local recovery login", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__LOCAL_LOGIN", "true")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.True(t, cfg.LocalLogin)
	})
}

func TestAuthFlags(t *testing.T) {
	t.Run("defaults to database managed", func(t *testing.T) {
		t.Setenv("KUMBUKA__AUTH_MODE", "")

		cfg, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
		})

		require.NoError(t, err)
		assert.Empty(t, cfg.AuthModeOverride)
	})

	t.Run("rejects unknown value", func(t *testing.T) {
		t.Parallel()

		_, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--auth-mode", "invalid",
		})

		require.Error(t, err)
	})

	t.Run("local override", func(t *testing.T) {
		t.Parallel()

		cfg, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--auth-mode", "local",
		})

		require.NoError(t, err)
		assert.Equal(t, domain.AuthModeLocal, cfg.AuthModeOverride)
	})

	t.Run("local override from environment", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__AUTH_MODE", "local")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.Equal(t, domain.AuthModeLocal, cfg.AuthModeOverride)
	})
}

func TestTrustedProxyFlags(t *testing.T) {
	t.Run("authorization overrides from environment", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__TRUSTED_GROUP_HEADERS", "X-Groups,X-Teams")
		t.Setenv("KUMBUKA__TRUSTED_ADMIN_GROUP", "platform-admins")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.Equal(
			t,
			[]string{"X-Groups", "X-Teams"},
			cfg.TrustedGroupHeaders,
		)
		assert.Equal(t, "platform-admins", cfg.TrustedAdminGroup)
	})
}

func TestOIDCFlags(t *testing.T) {
	t.Run("session secret and encryption key from environment", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv(
			"KUMBUKA__OIDC_SESSION_SECRET",
			"0123456789abcdef0123456789abcdef",
		)
		t.Setenv(
			"KUMBUKA__ENCRYPTION_KEY",
			"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		)

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.Equal(
			t,
			"0123456789abcdef0123456789abcdef",
			cfg.OIDCSessionSecret,
		)
		assert.Equal(
			t,
			"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
			cfg.EncryptionKey,
		)
	})

	t.Run("secrets remain deployment configuration", func(t *testing.T) {
		t.Parallel()

		cfg, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--oidc-client-secret", "client-secret",
			"--oidc-session-secret", "0123456789abcdef0123456789abcdef",
		})

		require.NoError(t, err)
		assert.Equal(t, "client-secret", cfg.OIDCClientSecret)
		assert.Equal(
			t,
			"0123456789abcdef0123456789abcdef",
			cfg.OIDCSessionSecret,
		)
	})

	t.Run("invalid encryption key is rejected", func(t *testing.T) {
		t.Parallel()

		_, err := parseTestConfig([]string{
			"--database-url", "postgres://example/kumbuka",
			"--encryption-key", "not-a-32-byte-base64-key",
		})

		require.Error(t, err)
	})

	t.Run("authorization overrides from environment", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__OIDC_GROUP_CLAIM", "roles")
		t.Setenv("KUMBUKA__OIDC_ADMIN_GROUP", "wiki-admins")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.Equal(t, "roles", cfg.OIDCGroupClaim)
		assert.Equal(t, "wiki-admins", cfg.OIDCAdminGroup)
	})
}

func TestLoggingFlags(t *testing.T) {
	t.Run("render timings from environment", func(t *testing.T) {
		t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
		t.Setenv("KUMBUKA__DEBUG_RENDER_TIMINGS", "true")

		cfg, err := parseTestConfig(nil)

		require.NoError(t, err)
		assert.True(t, cfg.DebugRenderTimings)
	})
}
