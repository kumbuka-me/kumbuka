package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// importTransactionKey isolates the current import transaction from unrelated context values.
type importTransactionKey struct{}

// importTransaction binds one PostgreSQL transaction to its owning store.
type importTransaction struct {
	// store is the connection owner that started the transaction.
	store *Store
	// tx is the active transaction used by every import repository operation.
	tx pgx.Tx
}

// importQuerier is the read and write subset shared by the pool and a transaction.
type importQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// importQuery uses the active import transaction when one belongs to this store.
func (s *Store) importQuery(ctx context.Context) importQuerier {
	if current, ok := ctx.Value(importTransactionKey{}).(importTransaction); ok && current.store == s {
		return current.tx
	}
	return s.pool
}

// activeImportTransaction returns a transaction started by this store, if present.
func (s *Store) activeImportTransaction(ctx context.Context) (pgx.Tx, bool) {
	current, ok := ctx.Value(importTransactionKey{}).(importTransaction)
	return current.tx, ok && current.store == s
}

// WithImportTransaction commits all resource, group, and page writes or rolls them back together.
func (s *Store) WithImportTransaction(ctx context.Context, run func(context.Context) error) error {
	if _, active := s.activeImportTransaction(ctx); active {
		return errors.New("nested portable import transaction")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ctx = context.WithValue(ctx, importTransactionKey{}, importTransaction{store: s, tx: tx})
	if err := run(ctx); err != nil {
		return err
	}
	return mutationError(tx.Commit(ctx))
}
