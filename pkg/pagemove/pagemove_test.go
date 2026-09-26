package pagemove

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	t.Parallel()

	oldSlug, newSlug, err := Normalize(" /Guide/Child/ ", "Archive / New Home", domain.MovePageOptions{})
	require.NoError(t, err)
	assert.Equal(t, "Guide/Child", oldSlug)
	assert.Equal(t, "archive/new-home", newSlug)
}

func TestNormalizeRejectsTreeMoveIntoItself(t *testing.T) {
	t.Parallel()

	_, _, err := Normalize("guide", "guide/child", domain.MovePageOptions{MoveChildren: true})

	require.Error(t, err)
}

func TestDestinationRejectsNoop(t *testing.T) {
	t.Parallel()

	_, _, err := Destination("archive/guide", "archive")

	require.Error(t, err)
}
