package service

import (
	"context"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// administrationRepository contains dashboard, tag, health, and audit operations.
type administrationRepository interface {
	Stats(context.Context) (domain.AdminStats, error)
	TagInfos(context.Context) ([]domain.TagInfo, error)
	DeleteTag(context.Context, int64) error
	DocumentationHealth(context.Context, time.Time) (domain.DocumentationHealth, error)
	AuditEvents(context.Context, int) ([]domain.AuditEvent, error)
}

// Administration exposes dashboard, documentation-health, tag, and audit use cases.
type Administration struct {
	// repository provides the persistence operations required by administration.
	repository administrationRepository
}

// NewAdministration constructs the administration service.
func NewAdministration(repository administrationRepository) *Administration {
	return &Administration{repository: repository}
}

// Stats returns aggregate counts for the administration dashboard.
func (s *Administration) Stats(ctx context.Context) (domain.AdminStats, error) {
	return s.repository.Stats(ctx)
}

// TagInfos returns tags with administration usage metadata.
func (s *Administration) TagInfos(ctx context.Context) ([]domain.TagInfo, error) {
	return s.repository.TagInfos(ctx)
}

// DeleteTag removes a tag and its page associations.
func (s *Administration) DeleteTag(ctx context.Context, id int64) error {
	return s.repository.DeleteTag(ctx, id)
}

// DocumentationHealth returns stale pages and broken links for administration.
func (s *Administration) DocumentationHealth(
	ctx context.Context,
	staleBefore time.Time,
) (domain.DocumentationHealth, error) {
	return s.repository.DocumentationHealth(ctx, staleBefore)
}

// AuditEvents returns recent administrative audit events.
func (s *Administration) AuditEvents(ctx context.Context, limit int) ([]domain.AuditEvent, error) {
	return s.repository.AuditEvents(ctx, limit)
}
