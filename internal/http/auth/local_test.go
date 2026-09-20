package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLocalSessionHash verifies regular local session tokens are hashed deterministically.
func TestLocalSessionHash(t *testing.T) {
	t.Parallel()

	assert.Equal(t, localSessionHash("token"), localSessionHash("token"))
	assert.NotEqual(t, localSessionHash("token"), localSessionHash("other"))
}
