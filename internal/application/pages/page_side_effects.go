package pages

import (
	"context"
	"log/slog"

	"github.com/kumbuka-me/kumbuka/internal/application/audit"
	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
)

// pageSideEffectRepository contains best-effort audit and notification persistence.
type pageSideEffectRepository interface {
	LogAudit(context.Context, int64, string, string, string, string) error
	NotifyMentions(context.Context, int64, string, string, string) error
	NotifyPageWatchers(context.Context, int64, string, string, string, string) error
}

// pageEffects owns best-effort side effects shared by page commands.
type pageEffects struct {
	// repository persists audit records and notification deliveries.
	repository pageSideEffectRepository
	// logger records side-effect failures without failing the primary mutation.
	logger *slog.Logger
	// eventSinks receive outgoing webhook events after successful mutations.
	eventSinks []webhooks.EventSink
}

// newPageEffects constructs page mutation side effects.
func newPageEffects(
	repository pageSideEffectRepository,
	logger *slog.Logger,
	eventSinks ...webhooks.EventSink,
) *pageEffects {
	if logger == nil {
		logger = slog.Default()
	}
	return &pageEffects{repository: repository, logger: logger, eventSinks: eventSinks}
}

// recordAudit is best effort after the primary mutation commits. Failures are observable, but must not turn a successful mutation into a retryable HTTP failure.
func (e *pageEffects) recordAudit(ctx context.Context, actorID int64, action, objectType, objectKey, detail string) {
	if e == nil || e.repository == nil {
		return
	}
	audit.Record(ctx, e.logger, e.repository, actorID, action, objectType, objectKey, detail)

	event := webhooks.OutgoingEvent{Event: action, ActorID: actorID, ObjectType: objectType, ObjectKey: objectKey, Detail: detail}
	for _, sink := range e.eventSinks {
		if err := sink.Emit(ctx, event); err != nil {
			e.logger.ErrorContext(ctx, "outgoing event delivery failed", "event", "page_side_effect_failed", "operation", "webhook", "action", action, "error", err)
		}
	}
}

// notifyMentions reports delivery failures without logging the page or comment body.
func (e *pageEffects) notifyMentions(ctx context.Context, actorID int64, body, title, destination string) {
	if e == nil || e.repository == nil {
		return
	}
	if err := e.repository.NotifyMentions(ctx, actorID, body, title, destination); err != nil {
		e.logger.ErrorContext(ctx,
			"page mentions failed",
			"event", "page_side_effect_failed",
			"operation", "notify_mentions",
			"actor_id", actorID,
			"destination", destination,
			"error", err,
		)
	}
}

// notifyWatchers reports delivery failures without changing the primary mutation result.
func (e *pageEffects) notifyWatchers(ctx context.Context, actorID int64, slug, title, body, destination string) {
	if e == nil || e.repository == nil {
		return
	}
	if err := e.repository.NotifyPageWatchers(ctx, actorID, slug, title, body, destination); err != nil {
		e.logger.ErrorContext(ctx,
			"page watch notifications failed",
			"event", "page_side_effect_failed",
			"operation", "notify_watchers",
			"actor_id", actorID,
			"slug", slug,
			"error", err,
		)
	}
}
