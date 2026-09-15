package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitPagePath(t *testing.T) {
	t.Parallel()

	parent, segment := splitPagePath(" platforms/containers/docker ")
	assert.Equal(t, "platforms/containers", parent)
	assert.Equal(t, "docker", segment)

	parent, segment = splitPagePath("reference")
	assert.Empty(t, parent)
	assert.Equal(t, "reference", segment)
}
