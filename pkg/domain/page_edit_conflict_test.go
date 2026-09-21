package domain

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPageEditConflictIncludesCurrentRevision(t *testing.T) {
	t.Parallel()
	assert.EqualError(t, &PageEditConflictError{CurrentRevision: 42}, "page changed while editing; current revision is 42")
}

func TestPageEditConflictWithoutRevision(t *testing.T) {
	t.Parallel()
	assert.EqualError(t, &PageEditConflictError{}, "page changed while editing")
}

func TestPageEditConflictWithNegativeRevision(t *testing.T) {
	t.Parallel()
	assert.EqualError(t, &PageEditConflictError{CurrentRevision: -1}, "page changed while editing")
}
