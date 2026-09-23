package access

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accessRepositoryStub provides controllable access repository behavior for tests.
type accessRepositoryStub struct {
	// batch configures the batch returned by the test double.
	batch map[string]domain.PageAccess
	// batchCalls counts batch calls observed by the test double.
	batchCalls int
	// singleCalls counts single calls observed by the test double.
	singleCalls int
	// batchErr configures the error returned by the test double.
	batchErr error
	// paths records the paths observed by the test double.
	paths []string
	// userID records the user ID observed by the test double.
	userID int64
	// access configures the access returned by the test double.
	access domain.PageAccess
	// path records the path observed by the test double.
	path string
	// group records the group observed by the test double.
	group int64
	// level records the level observed by the test double.
	level string
}

func (r *accessRepositoryStub) PageAccess(_ context.Context, path string, _ int64) (domain.PageAccess, error) {
	r.singleCalls++
	r.path = path
	return r.access, nil
}
func (*accessRepositoryStub) PageAccessRules(context.Context) ([]domain.PageAccessRule, error) {
	return nil, nil
}
func (r *accessRepositoryStub) SavePageAccessRule(_ context.Context, path string, groupID int64, access string) error {
	r.path, r.group, r.level = path, groupID, access
	return nil
}
func (*accessRepositoryStub) DeletePageAccessRule(context.Context, int64) error { return nil }

func TestPageAccess(t *testing.T) {
	t.Parallel()

	t.Run("unrestricted pages remain visible", func(t *testing.T) {
		t.Parallel()
		repository := &accessRepositoryStub{access: domain.PageAccess{}}
		allowed, err := NewAccess(repository).CanView(context.Background(), domain.User{ID: 2, Role: "viewer"}, "docs/start")
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("restricted pages require group access", func(t *testing.T) {
		t.Parallel()
		repository := &accessRepositoryStub{access: domain.PageAccess{Restricted: true}}
		allowed, err := NewAccess(repository).CanView(context.Background(), domain.User{ID: 2, Role: "viewer"}, "private/runbook")
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("edit grant requires editor role", func(t *testing.T) {
		t.Parallel()
		repository := &accessRepositoryStub{access: domain.PageAccess{Restricted: true, CanView: true, CanEdit: true}}
		viewerAllowed, err := NewAccess(repository).CanEdit(context.Background(), domain.User{ID: 2, Role: "viewer"}, "private/runbook")
		require.NoError(t, err)
		assert.False(t, viewerAllowed)
		editorAllowed, err := NewAccess(repository).CanEdit(context.Background(), domain.User{ID: 3, Role: "editor"}, "private/runbook")
		require.NoError(t, err)
		assert.True(t, editorAllowed)
	})

	t.Run("administrators bypass path rules", func(t *testing.T) {
		t.Parallel()
		allowed, err := NewAccess(nil).CanEdit(context.Background(), domain.User{Role: "admin"}, "private/runbook")
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("rules normalize paths", func(t *testing.T) {
		t.Parallel()
		repository := &accessRepositoryStub{}
		err := NewAccess(repository).SavePageAccessRule(context.Background(), "/Platform/Run Books/", 4, PageAccessEdit)
		require.NoError(t, err)
		assert.Equal(t, "platform/run-books", repository.path)
		assert.Equal(t, int64(4), repository.group)
		assert.Equal(t, PageAccessEdit, repository.level)
	})
}

func (r *accessRepositoryStub) PageAccessBatch(_ context.Context, paths []string, userID int64) (map[string]domain.PageAccess, error) {
	r.batchCalls++
	r.paths, r.userID = paths, userID
	return r.batch, r.batchErr
}

func TestFilterPagesUsesBulkAccess(t *testing.T) {
	t.Parallel()
	repository := &accessRepositoryStub{batch: map[string]domain.PageAccess{
		"open":    {},
		"allowed": {Restricted: true, CanView: true},
		"denied":  {Restricted: true},
	}}
	pages := []domain.Page{{Slug: "open"}, {Slug: "denied"}, {Slug: "Allowed"}, {Slug: "missing"}, {Slug: "open"}}
	result, err := NewAccess(repository).FilterPages(context.Background(), domain.User{ID: 42}, pages)
	require.NoError(t, err)
	assert.Equal(t, []domain.Page{pages[0], pages[2], pages[4]}, result)
	assert.Equal(t, 1, repository.batchCalls)
	assert.Zero(t, repository.singleCalls)
	assert.Equal(t, int64(42), repository.userID)
	assert.Equal(t, []string{"open", "denied", "allowed", "missing", "open"}, repository.paths)
	result[0].Title = "changed"
	assert.Empty(t, pages[0].Title)
}

func TestFilterPagesBypassAndFailure(t *testing.T) {
	t.Parallel()
	repository := &accessRepositoryStub{batchErr: errors.New("unavailable")}
	access := NewAccess(repository)
	pages := []domain.Page{{Slug: "private"}}
	result, err := access.FilterPages(context.Background(), domain.User{Role: "admin"}, pages)
	require.NoError(t, err)
	assert.Equal(t, pages, result)
	result[0].Slug = "changed"
	assert.Equal(t, "private", pages[0].Slug)
	_, err = access.FilterPages(context.Background(), domain.User{}, nil)
	require.NoError(t, err)
	assert.Zero(t, repository.batchCalls)
	result, err = access.FilterPages(context.Background(), domain.User{}, pages)
	require.ErrorIs(t, err, repository.batchErr)
	assert.Nil(t, result)
}
