package flags

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseTestConfig(args []string) (Config, error) {
	return Parse(args, "test")
}

func TestEnvironmentPrefix(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__AUTH_MODE", string(auth.AuthModeTrustedProxy))

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, "postgres://example/kumbuka", cfg.DatabaseURL)
	assert.Equal(t, auth.AuthModeTrustedProxy, cfg.AuthModeOverride)
}

func TestOIDCSessionSecretAndEncryptionKeyFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__OIDC_SESSION_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("KUMBUKA__ENCRYPTION_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", cfg.OIDCSessionSecret)
	assert.Equal(t, "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", cfg.EncryptionKey)
}

func TestAuthModeRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	_, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--auth-mode", "invalid",
	})

	require.Error(t, err)
}

func TestAuthModeDefaultsToDatabaseManaged(t *testing.T) {
	t.Parallel()

	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

	require.NoError(t, err)
	assert.Empty(t, cfg.AuthModeOverride)
}

func TestLocalRecoveryLoginCanBeEnabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__LOCAL_LOGIN", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.True(t, cfg.LocalLogin)
}

func TestUserRegistrationDefaultsToDatabaseManaged(t *testing.T) {
	t.Parallel()

	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka"})

	require.NoError(t, err)
	assert.Nil(t, cfg.AllowUserRegistrationOverride)
}

func TestUserRegistrationCanBeEnabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__ALLOW_USER_REGISTRATION", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	require.NotNil(t, cfg.AllowUserRegistrationOverride)
	assert.True(t, *cfg.AllowUserRegistrationOverride)
}

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

func TestRenderTimingsCanBeEnabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__DEBUG_RENDER_TIMINGS", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.True(t, cfg.DebugRenderTimings)
}

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

func TestEncryptionKeyRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	_, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--encryption-key", "not-a-32-byte-base64-key",
	})

	require.Error(t, err)
}

func TestPDFURLFromEnvironmentIncludesPath(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__PDF_URL", "http://html2pdf:8080/custom/render?profile=wiki")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, "http://html2pdf:8080/custom/render?profile=wiki", cfg.PDFURL)
}

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

func TestLocalAuthenticationOverride(t *testing.T) {
	t.Parallel()
	cfg, err := parseTestConfig([]string{"--database-url", "postgres://example/kumbuka", "--auth-mode", "local"})
	require.NoError(t, err)
	assert.Equal(t, auth.AuthModeLocal, cfg.AuthModeOverride)
}

func TestLocalAuthenticationOverrideFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__AUTH_MODE", "local")
	cfg, err := parseTestConfig(nil)
	require.NoError(t, err)
	assert.Equal(t, auth.AuthModeLocal, cfg.AuthModeOverride)
}

func TestUserRegistrationFlagRequiresExplicitValue(t *testing.T) {
	t.Parallel()

	_, err := parseTestConfig([]string{
		"--database-url", "postgres://example/kumbuka",
		"--allow-user-registration",
	})

	require.Error(t, err)
}

func TestReadOnlyCanBeEnabledFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__READ_ONLY", "true")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.True(t, cfg.ReadOnly)
}

func TestTrustedProxyAuthorizationOverridesFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__TRUSTED_GROUP_HEADERS", "X-Groups,X-Teams")
	t.Setenv("KUMBUKA__TRUSTED_ADMIN_GROUP", "platform-admins")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"X-Groups", "X-Teams"}, cfg.TrustedGroupHeaders)
	assert.Equal(t, "platform-admins", cfg.TrustedAdminGroup)
}

func TestOIDCAuthorizationOverridesFromEnvironment(t *testing.T) {
	t.Setenv("KUMBUKA__DATABASE_URL", "postgres://example/kumbuka")
	t.Setenv("KUMBUKA__OIDC_GROUP_CLAIM", "roles")
	t.Setenv("KUMBUKA__OIDC_ADMIN_GROUP", "wiki-admins")

	cfg, err := parseTestConfig(nil)

	require.NoError(t, err)
	assert.Equal(t, "roles", cfg.OIDCGroupClaim)
	assert.Equal(t, "wiki-admins", cfg.OIDCAdminGroup)
}
