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
	firstPlaintext, err := cipher.Decrypt(first)
	require.NoError(t, err)
	assert.Equal(t, "same secret", firstPlaintext)
	secondPlaintext, err := cipher.Decrypt(second)
	require.NoError(t, err)
	assert.Equal(t, "same secret", secondPlaintext)
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

func TestNewRejectsInvalidKey(t *testing.T) {
	t.Parallel()
	cipher, err := New("not-base64")
	require.Error(t, err)
	assert.Nil(t, cipher)
}

func TestNewTrimsKeyWhitespace(t *testing.T) {
	t.Parallel()
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := New(" \t" + key + "\n")
	require.NoError(t, err)
	assert.True(t, cipher.Configured())
}

func TestNilCipherRejectsOperations(t *testing.T) {
	t.Parallel()
	var cipher *Cipher
	assert.False(t, cipher.Configured())
	encrypted, err := cipher.Encrypt("secret")
	require.ErrorIs(t, err, ErrNotConfigured)
	assert.Empty(t, encrypted)
	decrypted, err := cipher.Decrypt("v1:anything")
	require.ErrorIs(t, err, ErrNotConfigured)
	assert.Empty(t, decrypted)
}

func TestCipherRoundTripsEmptyPlaintext(t *testing.T) {
	t.Parallel()
	cipher := newTestCipher(t)
	encrypted, err := cipher.Encrypt("")
	require.NoError(t, err)
	require.NotEmpty(t, encrypted)
	decrypted, err := cipher.Decrypt(encrypted)
	require.NoError(t, err)
	assert.Empty(t, decrypted)
}

func TestDecryptRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()
	plaintext, err := newTestCipher(t).Decrypt("v2:abc")
	require.EqualError(t, err, "decrypt application secret: unsupported ciphertext version")
	assert.Empty(t, plaintext)
}

func TestDecryptRejectsMalformedBase64(t *testing.T) {
	t.Parallel()
	plaintext, err := newTestCipher(t).Decrypt("v1:!")
	require.EqualError(t, err, "decrypt application secret: invalid ciphertext")
	assert.Empty(t, plaintext)
}

func TestDecryptRejectsTruncatedNonce(t *testing.T) {
	t.Parallel()
	cipher := newTestCipher(t)
	payload := make([]byte, cipher.aead.NonceSize()-1)
	plaintext, err := cipher.Decrypt("v1:" + base64.RawStdEncoding.EncodeToString(payload))
	require.EqualError(t, err, "decrypt application secret: invalid ciphertext")
	assert.Empty(t, plaintext)
}

func TestDecryptRejectsMissingAuthenticationTag(t *testing.T) {
	t.Parallel()
	cipher := newTestCipher(t)
	payload := make([]byte, cipher.aead.NonceSize())
	plaintext, err := cipher.Decrypt("v1:" + base64.RawStdEncoding.EncodeToString(payload))
	require.EqualError(t, err, "decrypt application secret: authentication failed")
	assert.Empty(t, plaintext)
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	t.Parallel()
	cipher := newTestCipher(t)
	encrypted, err := cipher.Encrypt("private secret")
	require.NoError(t, err)
	payload, err := base64.RawStdEncoding.DecodeString(encrypted[len(ciphertextVersion):])
	require.NoError(t, err)
	require.NotEmpty(t, payload)
	payload[len(payload)-1] ^= 1
	plaintext, err := cipher.Decrypt(ciphertextVersion + base64.RawStdEncoding.EncodeToString(payload))
	require.EqualError(t, err, "decrypt application secret: authentication failed")
	assert.Empty(t, plaintext)
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	t.Parallel()
	encrypted, err := newTestCipher(t).Encrypt("private secret")
	require.NoError(t, err)
	other, err := New(base64.StdEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789")))
	require.NoError(t, err)
	plaintext, err := other.Decrypt(encrypted)
	require.EqualError(t, err, "decrypt application secret: authentication failed")
	assert.Empty(t, plaintext)
}

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	cipher, err := New(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	require.NoError(t, err)
	return cipher
}
