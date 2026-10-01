package plugin

import (
	"context"
	"io/fs"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitingAssetStore exposes a cancellable storage read for lifecycle races.
type waitingAssetStore struct {
	Store
	entered chan context.Context
	release chan struct{}
}

func (s *waitingAssetStore) PluginPackage(ctx context.Context, id string) ([]byte, error) {
	s.entered <- ctx
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return s.Store.PluginPackage(ctx, id)
	}
}

func TestAssetReadDoesNotBlockInventoryOrShutdown(t *testing.T) {
	archive := previewTestArchive(t, previewTestPNG(t), nil)
	store := &waitingAssetStore{Store: testStore(t, archive, false), entered: make(chan context.Context, 1), release: make(chan struct{})}
	manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
	require.NoError(t, manager.Bootstrap(context.Background(), nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := manager.PluginPreview(ctx, "io.example.preview"); result <- err }()
	queryContext := <-store.entered
	_, bounded := queryContext.Deadline()
	assert.True(t, bounded)
	closed := make(chan error, 1)
	go func() { manager.Plugins(); closed <- manager.Close(context.Background()) }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(time.Second):
		cancel()
		t.Fatal("asset I/O held the manager lock")
	}
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
}

func TestAssetReadRejectsPackageRemovedDuringIO(t *testing.T) {
	archive := previewTestArchive(t, previewTestPNG(t), nil)
	store := &waitingAssetStore{Store: testStore(t, archive, false), entered: make(chan context.Context, 1), release: make(chan struct{})}
	manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
	require.NoError(t, manager.Bootstrap(context.Background(), nil))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := manager.PluginPreview(ctx, "io.example.preview"); result <- err }()
	<-store.entered
	require.NoError(t, manager.Close(ctx))
	close(store.release)
	require.ErrorIs(t, <-result, fs.ErrNotExist)
}
