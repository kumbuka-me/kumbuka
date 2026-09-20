package system

import (
	"context"
	"log/slog"

	"github.com/kumbuka-me/kumbuka/internal/application/audit"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// systemRepository contains startup and health persistence operations.
type systemRepository interface {
	audit.Repository
	Ping(context.Context) error
	SetupRequired(context.Context) (bool, error)
}

// System exposes application health and initial setup use cases.
type System struct {
	// repository provides the persistence operations required by system.
	repository systemRepository
	// logger reports failures from best-effort audit side effects.
	logger *slog.Logger
}

// NewSystem constructs the application health and setup service.
func NewSystem(repository systemRepository) *System {
	return &System{repository: repository, logger: audit.Logger(nil)}
}

// WithLogger uses logger for best-effort service side-effect failures.
func (s *System) WithLogger(logger *slog.Logger) *System {
	s.logger = audit.Logger(logger)
	return s
}

// Ping verifies that the application repository is reachable.
func (s *System) Ping(ctx context.Context) error {
	return s.repository.Ping(ctx)
}

// RecordSetupCompleted records creation of the initial administrator.
func (s *System) RecordSetupCompleted(ctx context.Context, actor domain.User) {
	audit.Record(
		ctx, s.logger, s.repository,
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
