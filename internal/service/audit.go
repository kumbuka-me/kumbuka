package service

import (
	"context"
)

// auditRepository records application audit events.
type auditRepository interface {
	LogAudit(context.Context, int64, string, string, string, string) error
}
