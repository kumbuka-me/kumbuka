package credential

import (
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// TestLocalPasswordHash verifies valid passwords are bcrypt-hashed and remain verifiable.
func TestLocalPasswordHash(t *testing.T) {
	t.Parallel()

	hash, err := HashLocalPassword("correct-horse-battery-staple")

	require.NoError(t, err)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct-horse-battery-staple")))
	assert.Error(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong-password")))
}

// TestLocalPasswordHashRejectsInvalidPassword verifies hashing reuses the shared password policy.
func TestLocalPasswordHashRejectsInvalidPassword(t *testing.T) {
	t.Parallel()

	_, err := HashLocalPassword("short")

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "Use at least 12 characters.", validation.UserMessage())
	assert.Equal(t, "validation failed", validation.Error())
}
