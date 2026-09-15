package secrets

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCipher(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := New(key)
	require.NoError(t, err)

	encrypted, err := cipher.Encrypt("Bearer secret-token")
	require.NoError(t, err)
	assert.NotContains(t, encrypted, "secret-token")
	assert.NotEqual(t, "Bearer secret-token", encrypted)

	decrypted, err := cipher.Decrypt(encrypted)
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret-token", decrypted)
}

func TestCipherUsesFreshNonce(t *testing.T) {
	t.Parallel()

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := New(key)
	require.NoError(t, err)

	first, err := cipher.Encrypt("same secret")
	require.NoError(t, err)
	second, err := cipher.Encrypt("same secret")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
}

func TestCipherRequiresConfiguredKey(t *testing.T) {
	t.Parallel()

	cipher, err := New("")
	require.NoError(t, err)

	_, err = cipher.Encrypt("secret")
	assert.ErrorIs(t, err, ErrNotConfigured)

	_, err = cipher.Decrypt("v1:anything")
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func TestValidateKey(t *testing.T) {
	t.Parallel()

	t.Run("empty key is optional", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, ValidateKey(""))
	})

	t.Run("accepts 32 byte base64 key", func(t *testing.T) {
		t.Parallel()

		value := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

		assert.NoError(t, ValidateKey(value))
	})

	t.Run("rejects malformed base64", func(t *testing.T) {
		t.Parallel()

		assert.Error(t, ValidateKey("not-base64"))
	})

	t.Run("rejects wrong key length", func(t *testing.T) {
		t.Parallel()

		value := base64.StdEncoding.EncodeToString([]byte("too-short"))

		assert.Error(t, ValidateKey(value))
	})
}
