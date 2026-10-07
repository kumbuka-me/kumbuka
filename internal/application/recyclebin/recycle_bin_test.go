package recyclebin

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recycleBinRepositoryStub provides controllable recycle-bin persistence for tests.
type recycleBinRepositoryStub struct {
	// deleteErr configures permanent deletion failure.
	deleteErr error
}

func (s *recycleBinRepositoryStub) DeletedPages(context.Context) ([]domain.DeletedPage, error) {
	return nil, nil
}

func (s *recycleBinRepositoryStub) RestorePage(context.Context, string) error { return nil }

func (s *recycleBinRepositoryStub) PermanentlyDeletePage(context.Context, string) error {
	return s.deleteErr
}

// navigationIconCacheStub records cache invalidations triggered by permanent deletion.
type navigationIconCacheStub struct {
	// calls counts cache invalidations observed by the test double.
	calls int
}

func (s *navigationIconCacheStub) InvalidateIcons() { s.calls++ }

func TestPermanentlyDeletePageInvalidatesNavigationIconsAfterPersistence(t *testing.T) {
	t.Parallel()

	repository := &recycleBinRepositoryStub{}
	cache := &navigationIconCacheStub{}
	recycleBin := NewRecycleBin(repository, cache)

	err := recycleBin.PermanentlyDeletePage(context.Background(), "guide")

	require.NoError(t, err)
	assert.Equal(t, 1, cache.calls)

	repository.deleteErr = errors.New("delete failed")
	err = recycleBin.PermanentlyDeletePage(context.Background(), "guide")

	require.Error(t, err)
	assert.Equal(t, 1, cache.calls)
}
