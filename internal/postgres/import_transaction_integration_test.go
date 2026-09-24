package postgres

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportTransactionRollsBackResourcesGroupsAndPages(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()

	actor, err := database.EnsureAdministrator(ctx, "import-transaction-admin", "", "Import Admin")
	require.NoError(t, err)
	name := fmt.Sprintf("import-transaction-%d", time.Now().UnixNano())
	failure := errors.New("later archive page failed")
	var imageID, attachmentID, groupID int64

	err = database.WithImportTransaction(ctx, func(txContext context.Context) error {
		image, err := database.SaveImage(txContext, name+".png", "image/png", []byte("image"), actor.ID)
		if err != nil {
			return err
		}
		imageID = image.ID
		attachment, err := database.SaveAttachment(txContext, name+".txt", "text/plain", []byte("text"), actor.ID)
		if err != nil {
			return err
		}
		attachmentID = attachment.ID
		group, err := database.CreateGroup(txContext, name)
		if err != nil {
			return err
		}
		groupID = group.ID
		_, err = database.SavePage(txContext, "", name, name, "", "", "body", "import", nil, nil,
			[]int64{groupID}, domain.PageMetadata{Status: "verified", OwnerGroupID: groupID}, nil, domain.PageRender{}, actor)
		if err != nil {
			return err
		}
		return failure
	})
	require.ErrorIs(t, err, failure)

	var present bool
	require.NoError(t, database.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM images WHERE id=$1)", imageID).Scan(&present))
	assert.False(t, present)
	require.NoError(t, database.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM attachments WHERE id=$1)", attachmentID).Scan(&present))
	assert.False(t, present)
	require.NoError(t, database.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM wiki_groups WHERE id=$1)", groupID).Scan(&present))
	assert.False(t, present)
	_, err = database.GetPage(ctx, name)
	require.ErrorIs(t, err, domain.ErrNotFound)
}
