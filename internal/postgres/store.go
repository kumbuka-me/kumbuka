package postgres

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store provides PostgreSQL-backed persistence for Kumbuka data.
type Store struct {
	// pool is the PostgreSQL connection pool used by store operations.
	pool *pgxpool.Pool
	// allowUserRegistrationOverride optionally replaces the persisted registration policy.
	allowUserRegistrationOverride *bool
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

// Close releases the PostgreSQL connection pool.
func (s *Store) Close() { s.pool.Close() }

// Ping verifies that PostgreSQL is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
