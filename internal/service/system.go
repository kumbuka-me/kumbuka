package service

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// systemRepository contains startup and health persistence operations.
type systemRepository interface {
	auditRepository
	Ping(context.Context) error
	SetupRequired(context.Context) (bool, error)
}

// System exposes application health and initial setup use cases.
type System struct {
	// repository provides the persistence operations required by system.
	repository systemRepository
}

// NewSystem constructs the application health and setup service.
func NewSystem(repository systemRepository) *System { return &System{repository: repository} }

// Ping verifies that the application repository is reachable.
func (s *System) Ping(ctx context.Context) error {
	return s.repository.Ping(ctx)
}

// RecordSetupCompleted records creation of the initial administrator.
func (s *System) RecordSetupCompleted(ctx context.Context, actor domain.User) {
	_ = s.repository.LogAudit(
		ctx,
		actor.ID,
		"setup.completed",
		"user",
		actor.Username,
		"Created initial local administrator",
	)
}

// SetupRequired reports whether the initial administrator must be created.
func (s *System) SetupRequired(ctx context.Context) (bool, error) {
	return s.repository.SetupRequired(ctx)
}
