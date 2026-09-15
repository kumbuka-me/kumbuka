// Package secrets encrypts sensitive application settings with a deployment-managed key.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	encryptionKeyBytes = 32
	ciphertextVersion  = "v1:"
)

// ErrNotConfigured indicates that no application encryption key is available.
var ErrNotConfigured = errors.New("application encryption key is not configured")

// Cipher encrypts and decrypts persisted application secrets.
type Cipher struct {
	aead cipher.AEAD
}

// ValidateKey validates an optional base64-encoded 256-bit encryption key.
func ValidateKey(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return errors.New("encryption key must be a base64-encoded 32-byte value")
	}
	if len(key) != encryptionKeyBytes {
		return errors.New("encryption key must be a base64-encoded 32-byte value")
	}

	return nil
}

// New constructs a cipher from an optional base64-encoded 256-bit key.
func New(value string) (*Cipher, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return &Cipher{}, nil
	}
	if err := ValidateKey(value); err != nil {
		return nil, err
	}

	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode application encryption key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create application secret cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create application secret cipher: %w", err)
	}

	return &Cipher{aead: aead}, nil
}

// Configured reports whether this process can encrypt and decrypt application secrets.
func (c *Cipher) Configured() bool {
	return c != nil && c.aead != nil
}

// Encrypt encrypts one plaintext value with a fresh random nonce.
func (c *Cipher) Encrypt(value string) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate application secret nonce: %w", err)
	}

	sealed := c.aead.Seal(nil, nonce, []byte(value), []byte(ciphertextVersion))
	payload := append(nonce, sealed...)

	return ciphertextVersion + base64.RawStdEncoding.EncodeToString(payload), nil
}

// Decrypt decrypts one versioned ciphertext value.
func (c *Cipher) Decrypt(value string) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	if !strings.HasPrefix(value, ciphertextVersion) {
		return "", errors.New("decrypt application secret: unsupported ciphertext version")
	}

	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, ciphertextVersion))
	if err != nil {
		return "", errors.New("decrypt application secret: invalid ciphertext")
	}
	if len(payload) < c.aead.NonceSize() {
		return "", errors.New("decrypt application secret: invalid ciphertext")
	}

	nonce := payload[:c.aead.NonceSize()]
	ciphertext := payload[c.aead.NonceSize():]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, []byte(ciphertextVersion))
	if err != nil {
		return "", errors.New("decrypt application secret: authentication failed")
	}

	return string(plaintext), nil
}
