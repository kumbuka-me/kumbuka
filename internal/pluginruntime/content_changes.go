package pluginruntime

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

const (
	contentChangeBatchSize   = 16
	contentChangeLease       = time.Minute
	contentChangePoll        = 5 * time.Second
	contentChangeMaxBackoff  = 15 * time.Minute
	contentChangeMaxErrorLen = 2048
)

// contentChangeRepository persists and leases committed page-source events.
type contentChangeRepository interface {
	EnqueuePluginContentChanges(context.Context, []domain.PluginContentChange) error
	ClaimPluginContentChanges(context.Context, int, time.Duration) ([]domain.PluginContentChange, error)
	CompletePluginContentChange(context.Context, int64) error
	RetryPluginContentChange(context.Context, int64, time.Time, string) error
}

// ContentChanges durably adapts committed page mutations to active plugin hooks.
type ContentChanges struct {
	// repository owns the durable delivery queue.
	repository contentChangeRepository
	// manager owns active plugin contributions.
	manager *plugin.Manager
	// notifications creates host-attributed in-app notifications.
	notifications plugincap.NotificationSender
	// logger records queue and delivery failures.
	logger *slog.Logger
	// wake requests an immediate queue drain after an enqueue.
	wake chan struct{}
}

// NewContentChanges constructs the durable post-commit plugin mutation adapter.
func NewContentChanges(
	repository contentChangeRepository,
	manager *plugin.Manager,
	notifications plugincap.NotificationSender,
	logger *slog.Logger,
) *ContentChanges {
	if logger == nil {
		logger = slog.Default()
	}
	return &ContentChanges{
		repository:    repository,
		manager:       manager,
		notifications: notifications,
		logger:        logger,
		wake:          make(chan struct{}, 1),
	}
}

// ContentChanged persists one delivery for every active plugin owning a committed-content hook.
func (c *ContentChanges) ContentChanged(ctx context.Context, change pages.PageContentChange) error {
	if c == nil || c.manager == nil {
		return nil
	}
	queued := c.queuedChanges(change)
	if len(queued) == 0 {
		return nil
	}
	if c.repository == nil {
		return c.deliverAll(ctx, queued)
	}
	if err := c.repository.EnqueuePluginContentChanges(ctx, queued); err != nil {
		deliveryErr := c.deliverAll(ctx, queued)
		if deliveryErr == nil {
			return nil
		}
		return errors.Join(err, deliveryErr)
	}
	c.signal()
	return nil
}

// queuedChanges snapshots the active content-change targets for one committed page mutation.
func (c *ContentChanges) queuedChanges(change pages.PageContentChange) []domain.PluginContentChange {
	targets := c.manager.ContentChangeTargets()
	queued := make([]domain.PluginContentChange, 0, len(targets))
	for _, target := range targets {
		queued = append(queued, domain.PluginContentChange{
			PluginID:         target.ID,
			Page:             change.Page,
			PreviousMarkdown: change.PreviousMarkdown,
			Markdown:         change.Markdown,
			ActorID:          change.Actor.ID,
		})
	}
	return queued
}

// Run drains durable content-change deliveries until the application context is canceled.
func (c *ContentChanges) Run(ctx context.Context) {
	if c == nil || c.repository == nil || c.manager == nil {
		return
	}
	ticker := time.NewTicker(contentChangePoll)
	defer ticker.Stop()

	c.processPending(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.wake:
			c.processPending(ctx)
		case <-ticker.C:
			c.processPending(ctx)
		}
	}
}

// signal wakes the background queue worker without blocking page saves.
func (c *ContentChanges) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// processPending claims bounded batches until no immediately deliverable events remain.
func (c *ContentChanges) processPending(ctx context.Context) {
	for ctx.Err() == nil {
		changes, err := c.repository.ClaimPluginContentChanges(ctx, contentChangeBatchSize, contentChangeLease)
		if err != nil {
			c.logger.ErrorContext(ctx, "plugin content change queue claim failed", "event", "plugin_content_change_queue_failed", "error", err)
			return
		}
		if len(changes) == 0 {
			return
		}
		for _, change := range changes {
			c.processOne(ctx, change)
		}
		if len(changes) < contentChangeBatchSize {
			return
		}
	}
}

// processOne delivers one leased event and either acknowledges or schedules it for retry.
func (c *ContentChanges) processOne(ctx context.Context, change domain.PluginContentChange) {
	if err := c.deliver(ctx, change); err != nil {
		retryAt := time.Now().Add(contentChangeRetryDelay(change.Attempts))
		message := boundedContentChangeError(err.Error())
		if retryErr := c.repository.RetryPluginContentChange(ctx, change.ID, retryAt, message); retryErr != nil {
			c.logger.ErrorContext(ctx, "plugin content change retry scheduling failed",
				"event", "plugin_content_change_queue_failed",
				"change_id", change.ID,
				"plugin_id", change.PluginID,
				"delivery_error", err,
				"retry_error", retryErr,
			)
			return
		}
		c.logger.WarnContext(ctx, "plugin content change delivery deferred",
			"event", "plugin_content_change_retry",
			"change_id", change.ID,
			"plugin_id", change.PluginID,
			"attempt", change.Attempts,
			"retry_at", retryAt,
			"error", err,
		)
		return
	}
	if err := c.repository.CompletePluginContentChange(ctx, change.ID); err != nil {
		c.logger.ErrorContext(ctx, "plugin content change acknowledgement failed",
			"event", "plugin_content_change_queue_failed",
			"change_id", change.ID,
			"plugin_id", change.PluginID,
			"error", err,
		)
	}
}

// deliverAll invokes every captured plugin target and combines fallback delivery failures.
func (c *ContentChanges) deliverAll(ctx context.Context, changes []domain.PluginContentChange) error {
	var combined error
	for _, change := range changes {
		combined = errors.Join(combined, c.deliver(ctx, change))
	}
	return combined
}

// deliver invokes the captured plugin's hook with only mutation-scoped external capabilities.
func (c *ContentChanges) deliver(ctx context.Context, change domain.PluginContentChange) error {
	return c.manager.ContentChangedFor(ctx, change.PluginID, plugin.ContentChangeRequest{
		Page:           plugincap.PageValue(change.Page),
		PreviousSource: change.PreviousMarkdown,
		Source:         change.Markdown,
	}, func(descriptor plugin.Descriptor) plugin.Context {
		return plugin.Context{
			Capabilities: plugincap.NotificationCapabilities(
				c.notifications,
				change.ActorID,
				descriptor.ID,
				descriptor.Name,
			),
			Features: c.manager.FeatureSettings(),
		}
	})
}

// contentChangeRetryDelay returns bounded exponential backoff from the current attempt count.
func contentChangeRetryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Second
	for attempt := 1; attempt < attempts && delay < contentChangeMaxBackoff; attempt++ {
		delay *= 2
		if delay >= contentChangeMaxBackoff {
			return contentChangeMaxBackoff
		}
	}
	return delay
}

// boundedContentChangeError limits persisted queue diagnostics while preserving useful context.
func boundedContentChangeError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= contentChangeMaxErrorLen {
		return message
	}
	limit := contentChangeMaxErrorLen
	for limit > 0 && !utf8.ValidString(message[:limit]) {
		limit--
	}
	return message[:limit]
}
