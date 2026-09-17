package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToPtr(t *testing.T) {
	t.Parallel()

	value := ToPtr("kumbuka")

	require.NotNil(t, value)
	assert.Equal(t, "kumbuka", *value)
}
