package bootstrap

import (
	"context"

	appportablearchive "github.com/kumbuka-me/kumbuka/internal/application/portablearchive"
	"github.com/kumbuka-me/kumbuka/internal/postgres"
)

// Ensure portableImportTransactionRunner satisfies the portable-import transaction boundary.
var _ appportablearchive.TransactionRunner = portableImportTransactionRunner{}

// portableImportTransactionRunner adapts PostgreSQL transactions to the portable-import boundary.
type portableImportTransactionRunner struct {
	// database owns the PostgreSQL pool used to start the import transaction.
	database *postgres.Store
}

// newPortableImportTransactionRunner constructs the PostgreSQL portable-import transaction adapter.
func newPortableImportTransactionRunner(database *postgres.Store) portableImportTransactionRunner {
	return portableImportTransactionRunner{database: database}
}

// Run executes work with a repository bound to one PostgreSQL transaction.
func (r portableImportTransactionRunner) Run(
	ctx context.Context,
	work func(appportablearchive.Repository) error,
) error {
	return r.database.WithImportTransaction(ctx, func(transaction *postgres.ImportStore) error {
		return work(transaction)
	})
}
