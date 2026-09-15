package service

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type accessRepositoryStub struct {
	access domain.PageAccess
	path   string
	group  int64
	level  string
}

func (r *accessRepositoryStub) PageAccess(_ context.Context, path string, _ int64) (domain.PageAccess, error) {
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
