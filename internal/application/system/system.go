package system

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/kumbuka-me/kumbuka/internal/application/audit"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// systemRepository contains startup and health persistence operations.
type systemRepository interface {
	audit.Repository
	DatabaseSize(context.Context) (int64, error)
	Ping(context.Context) error
}

// SetupState is the process-local view of the persisted one-time setup state.
type SetupState struct {
	// required is true until the initial administrator has been committed.
	required atomic.Bool
}

// NewSetupState creates the process-local setup state from the startup database check.
func NewSetupState(required bool) *SetupState {
	state := &SetupState{}
	state.required.Store(required)
	return state
}

// Required reports whether the one-time setup flow is still available.
func (s *SetupState) Required() bool {
	return s != nil && s.required.Load()
}

// Complete marks setup complete for the lifetime of the current process.
func (s *SetupState) Complete() {
	if s != nil {
		s.required.Store(false)
	}
}

// System exposes application health and initial setup use cases.
type System struct {
	// repository provides the persistence operations required by system.
	repository systemRepository
	// logger reports failures from best-effort audit side effects.
	logger *slog.Logger
	// setup is the process-local setup state initialized from PostgreSQL at startup.
	setup *SetupState
}

// NewSystem constructs the application health and setup service.
func NewSystem(repository systemRepository, logger *slog.Logger, setup *SetupState) *System {
	return &System{
		repository: repository,
		logger:     logger,
		setup:      setup,
	}
}

// DatabaseSize returns the current PostgreSQL database size in bytes.
func (s *System) DatabaseSize(ctx context.Context) (int64, error) {
	return s.repository.DatabaseSize(ctx)
}

// Ping verifies that the application repository is reachable.
func (s *System) Ping(ctx context.Context) error {
	return s.repository.Ping(ctx)
}

// RecordSetupCompleted records creation of the initial administrator.
func (s *System) RecordSetupCompleted(ctx context.Context, actor domain.User) {
	s.setup.Complete()
	audit.Record(
		ctx, s.logger, s.repository,
		actor.ID,
		"setup.completed",
		"user",
		actor.Username,
		"Created initial local administrator",
	)
}

// SetupRequired reports process-local setup state without performing I/O.
func (s *System) SetupRequired() bool {
	return s.setup.Required()
}
