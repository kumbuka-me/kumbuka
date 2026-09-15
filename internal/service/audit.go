package service

import (
	"context"
)

// auditRepository records application audit events.
type auditRepository interface {
	LogAudit(context.Context, int64, string, string, string, string) error
}

// audit writes an application audit event through a focused repository contract.
func audit(
	repository auditRepository,
	ctx context.Context,
	userID int64,
	action, objectType, objectKey, detail string,
) error {
	return repository.LogAudit(ctx, userID, action, objectType, objectKey, detail)
}
