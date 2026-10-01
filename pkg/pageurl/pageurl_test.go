package pageurl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPage(t *testing.T) {
	assert.Equal(t, "/p/123/guide/install", Page(123, "guide/install"))
}
