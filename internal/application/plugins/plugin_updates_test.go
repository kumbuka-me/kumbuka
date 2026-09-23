package plugins

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginUpdateClientStub provides controllable plugin update client behavior for tests.
type pluginUpdateClientStub struct {
	// refreshes configures or records the refreshes value used by the fixture.
	refreshes atomic.Int32
	// refreshErr configures the error returned by the test double.
	refreshErr error
	// updates configures or records the updates value used by the fixture.
	updates map[string]domain.PluginRelease
	// updatesErr configures the error returned by the test double.
	updatesErr error
	// archive configures or records the archive value used by the fixture.
	archive []byte
}

// Refresh records one catalog refresh and returns the configured error.
func (s *pluginUpdateClientStub) Refresh(context.Context) error {
	s.refreshes.Add(1)
	return s.refreshErr
}

// Updates returns the configured compatible releases.
func (s *pluginUpdateClientStub) Updates(map[string]string) (map[string]domain.PluginRelease, error) {
	return s.updates, s.updatesErr
}

// Download returns the configured package archive.
func (s *pluginUpdateClientStub) Download(context.Context, string, string) ([]byte, error) {
	return s.archive, nil
}

// pluginUpdateCatalogStub provides controllable plugin update catalog behavior for tests.
type pluginUpdateCatalogStub struct {
	// plugins configures or records the plugins value used by the fixture.
	plugins []plugin.LoadedPlugin
}

// Plugins returns a detached copy of the configured plugin inventory.
func (s pluginUpdateCatalogStub) Plugins() []plugin.LoadedPlugin {
	return append([]plugin.LoadedPlugin(nil), s.plugins...)
}

// pluginUpdateNotifierStub provides controllable plugin update notifier behavior for tests.
type pluginUpdateNotifierStub struct {
	// mu configures or records the mu value used by the fixture.
	mu sync.Mutex
	// notices configures or records the notices value used by the fixture.
	notices [][]domain.PluginUpdateNotice
	// err configures the error returned by the test double.
	err error
}

// NotifyPluginUpdates records one notification batch.
func (s *pluginUpdateNotifierStub) NotifyPluginUpdates(_ context.Context, notices []domain.PluginUpdateNotice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyNotices := append([]domain.PluginUpdateNotice(nil), notices...)
	s.notices = append(s.notices, copyNotices)
	return s.err
}

// snapshot returns detached copies of all recorded notification batches.
func (s *pluginUpdateNotifierStub) snapshot() [][]domain.PluginUpdateNotice {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([][]domain.PluginUpdateNotice, len(s.notices))
	for index := range s.notices {
		result[index] = append([]domain.PluginUpdateNotice(nil), s.notices[index]...)
	}
	return result
}

// TestPluginUpdatesRefreshDiscoversAndNotifies verifies plugin update service behavior.
func TestPluginUpdatesRefreshDiscoversAndNotifies(t *testing.T) {
	t.Parallel()

	const id = "me.kumbuka.callouts"
	client := &pluginUpdateClientStub{updates: map[string]domain.PluginRelease{
		id: {Version: "1.1.0"},
	}}
	notifier := &pluginUpdateNotifierStub{}
	service := NewPluginUpdates(
		client,
		pluginUpdateCatalogStub{plugins: []plugin.LoadedPlugin{{
			Manifest: pluginpackage.Manifest{ID: id, Name: "Callouts", Version: "1.0.0"},
		}}},
		notifier,
		15*time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	checkedAt := time.Date(2026, time.September, 18, 9, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return checkedAt }

	require.NoError(t, service.Refresh(context.Background()))

	assert.Equal(t, int32(1), client.refreshes.Load())
	notifications := notifier.snapshot()
	require.Len(t, notifications, 1)
	require.Len(t, notifications[0], 1)
	assert.Equal(t, domain.PluginUpdateNotice{
		ID:               id,
		Name:             "Callouts",
		CurrentVersion:   "1.0.0",
		AvailableVersion: "1.1.0",
	}, notifications[0][0])
	status := service.Status()
	assert.True(t, status.Automatic)
	assert.Equal(t, 15*time.Minute, status.Interval)
	assert.Equal(t, checkedAt, status.LastAttempt)
	assert.Equal(t, checkedAt, status.LastSuccess)
	assert.Empty(t, status.LastError)
}

// TestPluginUpdatesRefreshFailureRetainsLastSuccess verifies plugin update service behavior.
func TestPluginUpdatesRefreshFailureRetainsLastSuccess(t *testing.T) {
	t.Parallel()

	client := &pluginUpdateClientStub{updates: map[string]domain.PluginRelease{}}
	service := NewPluginUpdates(
		client,
		pluginUpdateCatalogStub{},
		nil,
		time.Hour,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	current := time.Date(2026, time.September, 18, 9, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return current }
	require.NoError(t, service.Refresh(context.Background()))

	client.refreshErr = errors.New("catalog offline")
	current = current.Add(time.Hour)
	require.Error(t, service.Refresh(context.Background()))

	status := service.Status()
	assert.Equal(t, current, status.LastAttempt)
	assert.Equal(t, current.Add(-time.Hour), status.LastSuccess)
	assert.Equal(t, "catalog offline", status.LastError)
}

// TestPluginUpdatesAvailableUsesLoadedInventory verifies handlers do not provide catalog state.
func TestPluginUpdatesAvailableUsesLoadedInventory(t *testing.T) {
	t.Parallel()

	const id = "me.kumbuka.callouts"
	client := &pluginUpdateClientStub{updates: map[string]domain.PluginRelease{id: {Version: "1.1.0"}}}
	service := NewPluginUpdates(
		client,
		pluginUpdateCatalogStub{plugins: []plugin.LoadedPlugin{{Manifest: pluginpackage.Manifest{ID: id, Version: "1.0.0"}}}},
		nil,
		0,
		nil,
	)

	updates, err := service.Available()

	require.NoError(t, err)
	assert.Equal(t, "1.1.0", updates[id].Version)
}

// TestPluginUpdatesRunChecksImmediatelyAndOnInterval verifies plugin update service behavior.
func TestPluginUpdatesRunChecksImmediatelyAndOnInterval(t *testing.T) {
	client := &pluginUpdateClientStub{updates: map[string]domain.PluginRelease{}}
	service := NewPluginUpdates(
		client,
		pluginUpdateCatalogStub{},
		nil,
		10*time.Millisecond,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go service.Run(ctx)

	require.Eventually(t, func() bool {
		return client.refreshes.Load() >= 2
	}, time.Second, 5*time.Millisecond)
}

// TestPluginUpdatesZeroIntervalDisablesSchedulerButKeepsManualRefresh verifies plugin update service behavior.
func TestPluginUpdatesZeroIntervalDisablesSchedulerButKeepsManualRefresh(t *testing.T) {
	t.Parallel()

	client := &pluginUpdateClientStub{updates: map[string]domain.PluginRelease{}}
	service := NewPluginUpdates(
		client,
		pluginUpdateCatalogStub{},
		nil,
		0,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	service.Run(context.Background())
	assert.Zero(t, client.refreshes.Load())
	assert.False(t, service.Status().Automatic)

	require.NoError(t, service.Refresh(context.Background()))
	assert.Equal(t, int32(1), client.refreshes.Load())
}
