package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// pageUpdateTransaction contains the shared ordered steps for applying a prepared page source update.
type pageUpdateTransaction struct {
	pageID           int64
	actorID          int64
	markdown         string
	message          string
	links            []string
	pluginUsage      any
	renderedContents json.RawMessage
	render           domain.PageRender
}

// applyPreparedPageUpdate writes one page update, revision, review state, caller-specific state, and links in that order.
func applyPreparedPageUpdate(
	ctx context.Context,
	tx pgx.Tx,
	update pageUpdateTransaction,
	afterReviews func(context.Context, pgx.Tx) error,
) error {
	if _, err := tx.Exec(ctx, `
UPDATE pages
SET markdown_content=$2,updated_by=$3,updated_at=now(),plugin_usage=$4::jsonb,
    rendered_html=$5,rendered_contents=$6::jsonb,render_fingerprint=$7,
    rendered_at=CASE WHEN $7<>'' THEN now() ELSE NULL END
WHERE id=$1`, update.pageID, update.markdown, update.actorID, update.pluginUsage, update.render.HTML, update.renderedContents, update.render.Fingerprint); err != nil {
		return mutationError(err)
	}
	if err := appendPageRevision(ctx, tx, update.pageID, update.markdown, update.message, update.actorID); err != nil {
		return mutationError(err)
	}
	if err := supersedePageReviews(ctx, tx, update.pageID); err != nil {
		return mutationError(err)
	}
	if afterReviews != nil {
		if err := afterReviews(ctx, tx); err != nil {
			return mutationError(err)
		}
	}
	if err := replacePageLinks(ctx, tx, update.pageID, update.links); err != nil {
		return mutationError(err)
	}
	return nil
}
