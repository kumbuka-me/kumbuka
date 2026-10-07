package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ImportStore exposes persistence operations bound to one portable-import transaction.
type ImportStore struct {
	// tx is the transaction used by every portable-import persistence operation.
	tx pgx.Tx
}

// WithImportTransaction commits all resource, group, and page writes or rolls them back together.
func (s *Store) WithImportTransaction(ctx context.Context, run func(*ImportStore) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := run(&ImportStore{tx: tx}); err != nil {
		return err
	}

	return mutationError(tx.Commit(ctx))
}
