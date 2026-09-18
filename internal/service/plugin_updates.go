package service

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

var errPluginUpdateCatalogUnavailable = errors.New("plugin update catalog is unavailable")

// pluginUpdateClient contains first-party catalog and package transport operations.
type pluginUpdateClient interface {
	// Refresh fetches and replaces the cached first-party catalog.
	Refresh(context.Context) error
	// Updates compares installed versions with the last successful catalog.
	Updates(map[string]string) (map[string]domain.PluginRelease, error)
	// Download retrieves and verifies one catalog release package by identity.
	Download(context.Context, string, string) ([]byte, error)
}

// pluginUpdateCatalog exposes the currently loaded plugin inventory.
type pluginUpdateCatalog interface {
	// Plugins returns the current loaded plugin inventory.
	Plugins() []plugin.LoadedPlugin
}

// pluginUpdateNotifier persists deduplicated administrator update notifications.
type pluginUpdateNotifier interface {
	// NotifyPluginUpdates creates durable deduplicated administrator notifications.
	NotifyPluginUpdates(context.Context, []domain.PluginUpdateNotice) error
}

// PluginUpdateStatus describes scheduler and catalog refresh state for administrators.
type PluginUpdateStatus struct {
	// Automatic reports whether scheduled catalog checks are enabled.
	Automatic bool
	// Interval is the configured scheduler interval.
	Interval time.Duration
	// LastAttempt records the most recent scheduled or manual refresh attempt.
	LastAttempt time.Time
	// LastSuccess records the most recent successful catalog refresh.
	LastSuccess time.Time
	// LastError contains the latest catalog refresh failure, if any.
	LastError string
}

// PluginUpdates coordinates scheduled discovery, manual refreshes, notifications, and downloads.
type PluginUpdates struct {
	// client owns catalog and package HTTP transport.
	client pluginUpdateClient
	// catalog exposes currently loaded plugins and versions.
	catalog pluginUpdateCatalog
	// notifier persists administrator notifications with durable deduplication.
	notifier pluginUpdateNotifier
	// interval controls scheduled catalog checks; zero disables the scheduler only.
	interval time.Duration
	// logger records background refresh and notification diagnostics.
	logger *slog.Logger
	// now supplies wall-clock timestamps for status reporting.
	now func() time.Time

	// refreshMu serializes scheduled and manual catalog refreshes.
	refreshMu sync.Mutex
	// statusMu protects refresh status exposed to handlers.
	statusMu sync.RWMutex
	// status stores the latest scheduler and refresh state.
	status PluginUpdateStatus
}

// NewPluginUpdates constructs the first-party plugin update application service.
func NewPluginUpdates(
	client pluginUpdateClient,
	catalog pluginUpdateCatalog,
	notifier pluginUpdateNotifier,
	interval time.Duration,
	logger *slog.Logger,
) *PluginUpdates {
	if logger == nil {
		logger = slog.Default()
	}

	return &PluginUpdates{
		client:   client,
		catalog:  catalog,
		notifier: notifier,
		interval: interval,
		logger:   logger,
		now:      time.Now,
		status: PluginUpdateStatus{
			Automatic: interval > 0,
			Interval:  interval,
		},
	}
}

// Run refreshes immediately and then on every configured interval until ctx is canceled.
func (s *PluginUpdates) Run(ctx context.Context) {
	if s == nil || s.interval <= 0 {
		return
	}

	s.refreshAndLog(ctx)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshAndLog(ctx)
		}
	}
}

// Refresh checks the first-party catalog immediately and bypasses the scheduled interval.
func (s *PluginUpdates) Refresh(ctx context.Context) error {
	if s == nil || s.client == nil {
		return errPluginUpdateCatalogUnavailable
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	attemptedAt := s.now()
	if err := s.client.Refresh(ctx); err != nil {
		s.recordRefresh(attemptedAt, err)
		return err
	}

	plugins := s.plugins()
	updates, err := s.client.Updates(pluginVersions(plugins))
	if err != nil {
		s.recordRefresh(attemptedAt, err)
		return err
	}
	s.recordRefresh(attemptedAt, nil)

	notices := pluginUpdateNotices(plugins, updates)
	if len(notices) != 0 && s.notifier != nil {
		if err := s.notifier.NotifyPluginUpdates(ctx, notices); err != nil {
			s.logger.Error(
				"notify administrators about plugin updates",
				"event", "plugin_update_notification_failed",
				"error", err,
			)
		}
	}

	return nil
}

// Available returns newer compatible releases for the currently loaded plugins.
func (s *PluginUpdates) Available() (map[string]domain.PluginRelease, error) {
	if s == nil || s.client == nil {
		return nil, errPluginUpdateCatalogUnavailable
	}

	return s.client.Updates(pluginVersions(s.plugins()))
}

// Download retrieves and verifies one selected plugin release by ID and version.
func (s *PluginUpdates) Download(ctx context.Context, id, version string) ([]byte, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("plugin update service is unavailable")
	}

	return s.client.Download(ctx, id, version)
}

// Status returns a snapshot of scheduler and catalog refresh state.
func (s *PluginUpdates) Status() PluginUpdateStatus {
	if s == nil {
		return PluginUpdateStatus{}
	}

	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.status
}

// plugins returns a snapshot of the currently loaded plugin inventory.
func (s *PluginUpdates) plugins() []plugin.LoadedPlugin {
	if s == nil || s.catalog == nil {
		return nil
	}

	return s.catalog.Plugins()
}

// refreshAndLog performs one scheduled refresh without terminating the scheduler after failures.
func (s *PluginUpdates) refreshAndLog(ctx context.Context) {
	if err := s.Refresh(ctx); err != nil && !errors.Is(err, context.Canceled) {
		s.logger.Warn(
			"check plugin update catalog",
			"event", "plugin_catalog_check_failed",
			"error", err,
		)
	}
}

// recordRefresh records the latest attempt while retaining the previous successful catalog timestamp after failures.
func (s *PluginUpdates) recordRefresh(attemptedAt time.Time, err error) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()

	s.status.LastAttempt = attemptedAt
	if err != nil {
		s.status.LastError = err.Error()
		return
	}

	s.status.LastSuccess = attemptedAt
	s.status.LastError = ""
}

// pluginVersions indexes loaded plugin versions by stable plugin ID.
func pluginVersions(items []plugin.LoadedPlugin) map[string]string {
	versions := make(map[string]string, len(items))
	for _, item := range items {
		versions[item.Manifest.ID] = item.Manifest.Version
	}
	return versions
}

// pluginUpdateNotices converts discovered releases into stable administrator notification data.
func pluginUpdateNotices(items []plugin.LoadedPlugin, updates map[string]domain.PluginRelease) []domain.PluginUpdateNotice {
	byID := make(map[string]plugin.LoadedPlugin, len(items))
	for _, item := range items {
		byID[item.Manifest.ID] = item
	}

	notices := make([]domain.PluginUpdateNotice, 0, len(updates))
	for id, release := range updates {
		item, ok := byID[id]
		if !ok {
			continue
		}
		notices = append(notices, domain.PluginUpdateNotice{
			ID:               id,
			Name:             item.Manifest.Name,
			CurrentVersion:   item.Manifest.Version,
			AvailableVersion: release.Version,
		})
	}
	sort.Slice(notices, func(i, j int) bool { return notices[i].ID < notices[j].ID })
	return notices
}
