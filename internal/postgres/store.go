package postgres

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides PostgreSQL-backed persistence for Kumbuka data.
type Store struct {
	// pool is the PostgreSQL connection pool used by store operations.
	pool *pgxpool.Pool
	// allowUserRegistrationOverride optionally replaces the persisted registration policy.
	allowUserRegistrationOverride *bool
}

// PoolStats is a transport-neutral snapshot of the PostgreSQL connection pool.
type PoolStats struct {
	// AcquiredConnections is the number of connections currently checked out by callers.
	AcquiredConnections int32
	// IdleConnections is the number of currently idle connections in the pool.
	IdleConnections int32
	// ConstructingConnections is the number of connections currently being established.
	ConstructingConnections int32
	// TotalConnections is the total number of acquired, idle, and constructing connections.
	TotalConnections int32
	// MaxConnections is the configured upper bound for the pool.
	MaxConnections int32
	// Acquires is the cumulative number of successful pool acquisitions.
	Acquires int64
	// EmptyAcquires is the cumulative number of successful acquisitions that had to wait because the pool was empty.
	EmptyAcquires int64
	// CanceledAcquires is the cumulative number of acquisitions canceled by their context.
	CanceledAcquires int64
	// AcquireDuration is the cumulative duration spent successfully acquiring connections.
	AcquireDuration time.Duration
	// EmptyAcquireWaitTime is the cumulative time successful callers waited while the pool was empty.
	EmptyAcquireWaitTime time.Duration
	// NewConnections is the cumulative number of connections opened by the pool.
	NewConnections int64
	// MaxIdleDestroyed is the cumulative number of connections closed after exceeding the idle limit.
	MaxIdleDestroyed int64
	// MaxLifetimeDestroyed is the cumulative number of connections closed after exceeding their lifetime.
	MaxLifetimeDestroyed int64
}

// Option customizes Store behavior when a database connection is opened.
type Option func(*Store)

// WithUserRegistrationOverride forces the effective external-user registration policy.
func WithUserRegistrationOverride(allowed bool) Option {
	return func(store *Store) {
		value := allowed
		store.allowUserRegistrationOverride = &value
	}
}

// Open connects to PostgreSQL and applies pending embedded migrations.
func Open(ctx context.Context, url string, logger *slog.Logger, options ...Option) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}

	s := &Store{pool: pool}
	for _, option := range options {
		option(s)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := s.migrate(ctx, logger); err != nil {
		pool.Close()
		return nil, err
	}

	return s, nil
}

// userRegistrationOverride returns the deployment-managed registration value when configured.
func (s *Store) userRegistrationOverride() (bool, bool) {
	if s.allowUserRegistrationOverride == nil {
		return false, false
	}

	return *s.allowUserRegistrationOverride, true
}

// PoolStats returns a point-in-time snapshot of PostgreSQL connection-pool activity.
func (s *Store) PoolStats() PoolStats {
	stats := s.pool.Stat()

	return PoolStats{
		AcquiredConnections:     stats.AcquiredConns(),
		IdleConnections:         stats.IdleConns(),
		ConstructingConnections: stats.ConstructingConns(),
		TotalConnections:        stats.TotalConns(),
		MaxConnections:          stats.MaxConns(),
		Acquires:                stats.AcquireCount(),
		EmptyAcquires:           stats.EmptyAcquireCount(),
		CanceledAcquires:        stats.CanceledAcquireCount(),
		AcquireDuration:         stats.AcquireDuration(),
		EmptyAcquireWaitTime:    stats.EmptyAcquireWaitTime(),
		NewConnections:          stats.NewConnsCount(),
		MaxIdleDestroyed:        stats.MaxIdleDestroyCount(),
		MaxLifetimeDestroyed:    stats.MaxLifetimeDestroyCount(),
	}
}

// Close releases the PostgreSQL connection pool.
func (s *Store) Close() { s.pool.Close() }

// Ping verifies that PostgreSQL is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
