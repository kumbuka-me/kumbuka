package service

import (
	"context"
	"log/slog"
)

// auditRepository records application audit events.
type auditRepository interface {
	LogAudit(context.Context, int64, string, string, string, string) error
}

// serviceLogger returns a usable logger for services with best-effort side effects.
func serviceLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}

	return logger
}

// recordAuditEvent records an audit event without turning an already-committed
// primary mutation into a retryable error. Persistence failures remain visible
// through structured logs.
func recordAuditEvent(
	ctx context.Context,
	logger *slog.Logger,
	repository auditRepository,
	actorID int64,
	action, objectType, objectKey, detail string,
) {
	if err := repository.LogAudit(ctx, actorID, action, objectType, objectKey, detail); err != nil {
		serviceLogger(logger).ErrorContext(ctx,
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
