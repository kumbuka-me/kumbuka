package auth

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalPasswordHash(t *testing.T) {
	t.Parallel()

	hash, err := HashLocalPassword("correct-horse-battery-staple")

	require.NoError(t, err)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("correct-horse-battery-staple")))
	assert.Error(t, bcrypt.CompareHashAndPassword([]byte(hash), []byte("wrong-password")))
}

func TestLocalPasswordHashRejectsInvalidPassword(t *testing.T) {
	t.Parallel()

	_, err := HashLocalPassword("short")

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "Use at least 12 characters.", validation.UserMessage())
	assert.Equal(t, "validation failed", validation.Error())
}

func TestLocalSessionHash(t *testing.T) {
	t.Parallel()

	assert.Equal(t, localSessionHash("token"), localSessionHash("token"))
	assert.NotEqual(t, localSessionHash("token"), localSessionHash("other"))
}
