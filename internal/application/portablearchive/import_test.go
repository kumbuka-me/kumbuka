package portablearchive

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/portable"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// importRepositoryStub supplies the transaction-scoped operations exercised by empty archive fixtures.
type importRepositoryStub struct {
	Repository
	groupsErr error
}

// Groups returns the configured group lookup result.
func (s *importRepositoryStub) Groups(context.Context) ([]domain.Group, error) {
	return nil, s.groupsErr
}

// importProgressStub records committed portable imports.
type importProgressStub struct {
	calls int
	count int
}

// navigationIconCacheStub records post-commit cache invalidations.
type navigationIconCacheStub struct {
	calls int
}

func (s *navigationIconCacheStub) InvalidateIcons() { s.calls++ }

type transactionRunnerStub func(context.Context, func(Repository) error) error

func (run transactionRunnerStub) Run(ctx context.Context, work func(Repository) error) error {
	return run(ctx, work)
}

// RecordPortableImport records one committed import result.
func (s *importProgressStub) RecordPortableImport(_ context.Context, _ domain.User, count int) {
	s.calls++
	s.count = count
}

func TestImporterRecordsProgressAfterCommit(t *testing.T) {
	progress := &importProgressStub{}
	cache := &navigationIconCacheStub{}
	transactionCalls := 0
	importer := NewImporter(
		transactionRunnerStub(func(ctx context.Context, run func(Repository) error) error {
			transactionCalls++
			return run(&importRepositoryStub{})
		}),
		nil,
		progress,
		cache,
	)

	count, err := importer.Restore(context.Background(), portable.Archive{}, domain.User{ID: 7})

	require.NoError(t, err)
	assert.Zero(t, count)
	assert.Equal(t, 1, transactionCalls)
	assert.Equal(t, 1, cache.calls)
	assert.Equal(t, 1, progress.calls)
	assert.Zero(t, progress.count)
}

func TestImporterDoesNotRecordProgressWhenRestoreFails(t *testing.T) {
	progress := &importProgressStub{}
	cache := &navigationIconCacheStub{}
	wantErr := errors.New("load groups")
	importer := NewImporter(
		transactionRunnerStub(func(_ context.Context, run func(Repository) error) error {
			return run(&importRepositoryStub{groupsErr: wantErr})
		}),
		nil,
		progress,
		cache,
	)

	count, err := importer.Restore(context.Background(), portable.Archive{}, domain.User{ID: 7})

	require.ErrorIs(t, err, wantErr)
	assert.Zero(t, count)
	assert.Zero(t, cache.calls)
	assert.Zero(t, progress.calls)
}

func TestImporterDoesNotRecordProgressWhenCommitFails(t *testing.T) {
	progress := &importProgressStub{}
	cache := &navigationIconCacheStub{}
	wantErr := errors.New("commit transaction")
	importer := NewImporter(
		transactionRunnerStub(func(_ context.Context, run func(Repository) error) error {
			if err := run(&importRepositoryStub{}); err != nil {
				return err
			}
			return wantErr
		}),
		nil,
		progress,
		cache,
	)

	count, err := importer.Restore(context.Background(), portable.Archive{}, domain.User{ID: 7})

	require.ErrorIs(t, err, wantErr)
	assert.Zero(t, count)
	assert.Zero(t, cache.calls)
	assert.Zero(t, progress.calls)
}
