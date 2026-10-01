package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageMovesPreserveEveryHistoricalSlug(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)
	user, err := database.EnsureAdministrator(ctx, "page-moves", "", "Page Moves")
	require.NoError(t, err)

	root := insertMoveTestPage(t, ctx, database, "foo", user.ID)
	child := insertMoveTestPage(t, ctx, database, "foo/child", user.ID)
	require.NoError(t, database.MovePage(ctx, "foo", "bar", domain.MovePageOptions{MoveChildren: true}, user))
	require.NoError(t, database.MovePage(ctx, "bar", "baz", domain.MovePageOptions{MoveChildren: true}, user))

	assert.Equal(t, root, aliasPageID(t, ctx, database, "foo"))
	assert.Equal(t, root, aliasPageID(t, ctx, database, "bar"))
	assert.Equal(t, child, aliasPageID(t, ctx, database, "foo/child"))
	assert.Equal(t, child, aliasPageID(t, ctx, database, "bar/child"))

	require.NoError(t, database.MovePage(ctx, "baz", "foo", domain.MovePageOptions{MoveChildren: true}, user))
	assert.Equal(t, root, currentPageID(t, ctx, database, "foo"))
	assert.Equal(t, child, currentPageID(t, ctx, database, "foo/child"))
	assertAliasMissing(t, ctx, database, "foo")
	assertAliasMissing(t, ctx, database, "foo/child")
	assert.Equal(t, root, aliasPageID(t, ctx, database, "bar"))
	assert.Equal(t, root, aliasPageID(t, ctx, database, "baz"))
	assert.Equal(t, child, aliasPageID(t, ctx, database, "bar/child"))
	assert.Equal(t, child, aliasPageID(t, ctx, database, "baz/child"))
}

func TestPageMoveRejectsAliasOwnedByAnotherPage(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)
	user, err := database.EnsureAdministrator(ctx, "page-move-collision", "", "Page Move Collision")
	require.NoError(t, err)

	insertMoveTestPage(t, ctx, database, "source", user.ID)
	target := insertMoveTestPage(t, ctx, database, "target-owner", user.ID)
	_, err = database.pool.Exec(ctx, `INSERT INTO page_aliases(alias,page_id) VALUES('reserved', $1)`, target)
	require.NoError(t, err)

	err = database.MovePage(ctx, "source", "reserved", domain.MovePageOptions{}, user)
	require.ErrorIs(t, err, domain.ErrAlreadyExists)
}

func TestPageMoveRejectsDestinationAliasOwnedByAnotherMovingPage(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)
	user, err := database.EnsureAdministrator(ctx, "page-move-subtree-collision", "", "Page Move Subtree Collision")
	require.NoError(t, err)

	root := insertMoveTestPage(t, ctx, database, "foo", user.ID)
	insertMoveTestPage(t, ctx, database, "foo/child", user.ID)
	_, err = database.pool.Exec(ctx, `INSERT INTO page_aliases(alias,page_id) VALUES('bar/child', $1)`, root)
	require.NoError(t, err)

	err = database.MovePage(ctx, "foo", "bar", domain.MovePageOptions{MoveChildren: true}, user)
	require.ErrorIs(t, err, domain.ErrAlreadyExists)
}

func TestRenamePageRecordReusesItsOwnAlias(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)
	user, err := database.EnsureAdministrator(ctx, "page-rename-alias", "", "Page Rename Alias")
	require.NoError(t, err)

	pageID := insertMoveTestPage(t, ctx, database, "foo", user.ID)
	renameMoveTestPage(t, ctx, database, pageID, "foo", "bar")
	renameMoveTestPage(t, ctx, database, pageID, "bar", "foo")

	assert.Equal(t, pageID, currentPageID(t, ctx, database, "foo"))
	assertAliasMissing(t, ctx, database, "foo")
	assert.Equal(t, pageID, aliasPageID(t, ctx, database, "bar"))
}

func renameMoveTestPage(t *testing.T, ctx context.Context, database *Store, pageID int64, oldSlug, newSlug string) {
	t.Helper()
	tx, err := database.pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	require.NoError(t, renamePageRecord(ctx, tx, pageID, oldSlug, newSlug))
	require.NoError(t, tx.Commit(ctx))
}

func currentPageID(t *testing.T, ctx context.Context, database *Store, slug string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT id FROM pages WHERE slug=$1 AND deleted_at IS NULL`, slug).Scan(&id))
	return id
}

func assertAliasMissing(t *testing.T, ctx context.Context, database *Store, alias string) {
	t.Helper()
	var exists bool
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM page_aliases WHERE alias=$1)`, alias).Scan(&exists))
	assert.False(t, exists, "current page slug must not also remain an alias")
}

func insertMoveTestPage(t *testing.T, ctx context.Context, database *Store, slug string, userID int64) int64 {
	t.Helper()
	var id int64
	err := database.pool.QueryRow(ctx, `
INSERT INTO pages(slug,title,markdown_content,created_by,updated_by)
VALUES($1,$1,'',$2,$2)
RETURNING id`, slug, userID).Scan(&id)
	require.NoError(t, err)
	return id
}

func aliasPageID(t *testing.T, ctx context.Context, database *Store, alias string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT page_id FROM page_aliases WHERE alias=$1`, alias).Scan(&id))
	return id
}
