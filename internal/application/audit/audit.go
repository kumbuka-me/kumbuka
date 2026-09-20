package audit

import (
	"context"
	"log/slog"
)

// Repository records application audit events.
type Repository interface {
	LogAudit(context.Context, int64, string, string, string, string) error
}

// Logger returns a usable logger for services with best-effort side effects.
func Logger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}

	return logger
}

// Record records an audit event without turning an already-committed
// primary mutation into a retryable error. Persistence failures remain visible
// through structured logs.
func Record(
	ctx context.Context,
	logger *slog.Logger,
	repository Repository,
	actorID int64,
	action, objectType, objectKey, detail string,
) {
	if err := repository.LogAudit(ctx, actorID, action, objectType, objectKey, detail); err != nil {
		Logger(logger).ErrorContext(ctx,
			"record audit event",
			"event", "audit_record_failed",
			"action", action,
			"actor_id", actorID,
			"object_type", objectType,
			"object_key", objectKey,
			"error", err,
		)
	}
}
