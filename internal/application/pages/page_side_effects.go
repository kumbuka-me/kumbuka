package pages

import (
	"context"
	"log/slog"

	"github.com/kumbuka-me/kumbuka/internal/application/audit"
	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// pageSideEffectRepository contains persistence required to resolve page side-effect recipients.
type pageSideEffectRepository interface {
	LogAudit(context.Context, int64, string, string, string, string) error
	PageWatcherUserIDs(context.Context, int64, string) ([]int64, error)
}

// NotificationSender creates application-owned notifications and their configured delivery events.
type NotificationSender interface {
	SendMentions(context.Context, int64, string, string, string) error
	SendCore(context.Context, int64, int64, domain.NotificationKind, string, string, string) error
}

// pageEffects owns best-effort side effects shared by page commands.
type pageEffects struct {
	// repository persists audit records and resolves notification recipients.
	repository pageSideEffectRepository
	// logger records side-effect failures without failing the primary mutation.
	logger *slog.Logger
	// eventSinks receive outgoing webhook events after successful mutations.
	eventSinks []webhooks.EventSink
	// notifications creates core inbox items through the shared notification service.
	notifications NotificationSender
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

// withNotifications routes page notifications through the shared notification service.
func (e *pageEffects) withNotifications(sender NotificationSender) *pageEffects {
	if e != nil {
		e.notifications = sender
	}
	return e
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
	if e == nil || e.notifications == nil {
		return
	}
	if err := e.notifications.SendMentions(ctx, actorID, body, title, destination); err != nil {
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

// notifyUser sends one core notification as a best-effort application side effect.
func (e *pageEffects) notifyUser(
	ctx context.Context,
	recipientUserID, actorID int64,
	kind domain.NotificationKind,
	title, body, destination string,
) {
	if e == nil || e.notifications == nil || recipientUserID <= 0 {
		return
	}
	if err := e.notifications.SendCore(ctx, recipientUserID, actorID, kind, title, body, destination); err != nil {
		e.logger.ErrorContext(ctx,
			"page notification failed",
			"event", "page_side_effect_failed",
			"operation", "notify_user",
			"recipient_user_id", recipientUserID,
			"actor_id", actorID,
			"destination", destination,
			"error", err,
		)
	}
}

// notifyWatchers resolves watcher recipients in persistence and sends notifications through the shared application service.
func (e *pageEffects) notifyWatchers(ctx context.Context, actorID int64, slug, title, body, destination string) {
	if e == nil || e.repository == nil || e.notifications == nil {
		return
	}
	userIDs, err := e.repository.PageWatcherUserIDs(ctx, actorID, slug)
	if err != nil {
		e.logger.ErrorContext(ctx,
			"page watch recipients failed",
			"event", "page_side_effect_failed",
			"operation", "resolve_watchers",
			"actor_id", actorID,
			"slug", slug,
			"error", err,
		)
		return
	}
	for _, userID := range userIDs {
		e.notifyUser(ctx, userID, actorID, domain.NotificationKindWatch, title, body, destination)
	}
}
