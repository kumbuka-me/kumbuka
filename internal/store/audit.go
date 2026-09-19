package store

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// LogAudit records one user action for administrative traceability.
func (s *Store) LogAudit(ctx context.Context, userID int64, action, objectType, objectKey, detail string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO audit_events(user_id,action,object_type,object_key,detail)
VALUES(NULLIF($1,0),$2,$3,$4,$5)`, userID, action, objectType, objectKey, detail)
	return err
}

// AuditEvents returns recent audit events newest first.
func (s *Store) AuditEvents(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	rows, err := s.pool.Query(ctx, `
SELECT e.id,coalesce(u.display_name,u.username,'System'),e.action,e.object_type,e.object_key,e.detail,e.created_at
FROM audit_events e
LEFT JOIN users u ON u.id=e.user_id
ORDER BY e.created_at DESC,e.id DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var events []domain.AuditEvent

	for rows.Next() {
		var event domain.AuditEvent
		if err := rows.Scan(&event.ID, &event.Actor, &event.Action, &event.ObjectType, &event.ObjectKey, &event.Detail, &event.CreatedAt); err != nil {
			return nil, err
		}

		events = append(events, event)
	}

	return events, rows.Err()
}
