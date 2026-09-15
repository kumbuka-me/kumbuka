package service

import "context"

// recordAudit is best effort after the primary mutation commits. Failures are
// observable, but must not turn a successful mutation into a retryable HTTP failure.
func (s *Pages) recordAudit(ctx context.Context, actorID int64, action, objectType, objectKey, detail string) {
	if err := s.repository.LogAudit(ctx, actorID, action, objectType, objectKey, detail); err != nil {
		s.logger.ErrorContext(ctx,
			"page audit failed",
			"event", "page_side_effect_failed",
			"operation", "audit",
			"action", action,
			"actor_id", actorID,
			"object_type", objectType,
			"object_key", objectKey,
			"error", err,
		)
	}

	event := OutgoingEvent{Event: action, ActorID: actorID, ObjectType: objectType, ObjectKey: objectKey, Detail: detail}
	for _, sink := range s.eventSinks {
		if err := sink.Emit(ctx, event); err != nil {
			s.logger.ErrorContext(ctx, "outgoing event delivery failed", "event", "page_side_effect_failed", "operation", "webhook", "action", action, "error", err)
		}
	}
}

// notifyMentions reports delivery failures without logging the page or comment body.
func (s *Pages) notifyMentions(ctx context.Context, actorID int64, body, title, destination string) {
	if err := s.repository.NotifyMentions(ctx, actorID, body, title, destination); err != nil {
		s.logger.ErrorContext(ctx,
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
func (s *Pages) notifyWatchers(ctx context.Context, actorID int64, slug, title, body, destination string) {
	if err := s.repository.NotifyPageWatchers(ctx, actorID, slug, title, body, destination); err != nil {
		s.logger.ErrorContext(ctx,
			"page watch notifications failed",
			"event", "page_side_effect_failed",
			"operation", "notify_watchers",
			"actor_id", actorID,
			"slug", slug,
			"error", err,
		)
	}
}
