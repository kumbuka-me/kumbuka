package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

// pageSaveRecord contains the normalized values persisted in the pages table.
type pageSaveRecord struct {
	// previousSlug identifies the existing page before a rename.
	previousSlug string
	// slug is the requested canonical page path.
	slug string
	// title is the page title.
	title string
	// language is the page content-language identifier.
	language string
	// markdown is the canonical Markdown source.
	markdown string
	// metadata contains lifecycle, ownership, and plugin-usage metadata.
	metadata domain.PageMetadata
	// render contains the reusable rendered page artifact.
	render domain.PageRender
	// pluginUsage is the JSON-ready plugin-usage value stored with the page.
	pluginUsage any
	// renderedContents is the JSON-encoded rendered contents metadata.
	renderedContents json.RawMessage
	// userID identifies the user performing the save.
	userID int64
}

// pageSaveTransaction contains the complete ordered page mutation applied inside one transaction.
type pageSaveTransaction struct {
	// record contains the pages-table values and derived render metadata.
	record pageSaveRecord
	// slug is the canonical page path used by related metadata tables.
	slug string
	// icon is the navigation icon assigned to the page.
	icon string
	// markdown is the source captured in the new revision.
	markdown string
	// message is the revision message.
	message string
	// tags are the complete tag set replacing persisted associations.
	tags []string
	// groupIDs are the complete explicit page-group assignments.
	groupIDs []int64
	// properties are the complete page property set.
	properties map[string]string
	// links are the complete outgoing link set.
	links []string
	// user is the actor whose assignment permissions and identity apply.
	user domain.User
}

// SavePage persists page content, revision history, tags, and links transactionally.
func (s *Store) SavePage(
	ctx context.Context,
	previousSlug, slug, title, icon, language, markdown, message string,
	tags, links []string,
	groupIDs []int64,
	metadata domain.PageMetadata,
	properties map[string]string,
	render domain.PageRender,
	user domain.User,
) (domain.Page, error) {
	mutation, err := preparePageSaveTransaction(
		previousSlug, slug, title, icon, language, markdown, message, tags, links, groupIDs, metadata, properties, render, user,
	)
	if err != nil {
		return domain.Page{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Page{}, mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	page, err := persistPageSaveTransaction(ctx, tx, mutation)
	if err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Page{}, mutationError(err)
	}

	return page, nil
}

// SavePage persists page content inside the caller-owned import transaction.
func (s *ImportStore) SavePage(
	ctx context.Context,
	previousSlug, slug, title, icon, language, markdown, message string,
	tags, links []string,
	groupIDs []int64,
	metadata domain.PageMetadata,
	properties map[string]string,
	render domain.PageRender,
	user domain.User,
) (domain.Page, error) {
	mutation, err := preparePageSaveTransaction(
		previousSlug, slug, title, icon, language, markdown, message, tags, links, groupIDs, metadata, properties, render, user,
	)
	if err != nil {
		return domain.Page{}, err
	}

	page, err := persistPageSaveTransaction(ctx, s.tx, mutation)
	if err != nil {
		return domain.Page{}, mutationError(err)
	}

	return page, nil
}

// persistPageSaveTransaction validates assignments, writes the page, and reloads it through one transaction.
func persistPageSaveTransaction(ctx context.Context, tx pgx.Tx, mutation pageSaveTransaction) (domain.Page, error) {
	if err := validateAssignableGroup(ctx, tx, mutation.record.metadata.OwnerGroupID, mutation.user); err != nil {
		return domain.Page{}, err
	}
	if err := executePageSaveTransaction(ctx, tx, mutation); err != nil {
		return domain.Page{}, err
	}

	return getPage(ctx, tx, mutation.slug)
}

// preparePageSaveTransaction validates derived data and packages the ordered transactional write.
func preparePageSaveTransaction(
	previousSlug, slug, title, icon, language, markdown, message string,
	tags, links []string,
	groupIDs []int64,
	metadata domain.PageMetadata,
	properties map[string]string,
	render domain.PageRender,
	user domain.User,
) (pageSaveTransaction, error) {
	metadata, pluginUsage, renderedContents, render, err := preparePageSave(metadata, render)
	if err != nil {
		return pageSaveTransaction{}, err
	}
	return pageSaveTransaction{
		record: pageSaveRecord{
			previousSlug: previousSlug, slug: slug, title: title, language: language, markdown: markdown,
			metadata: metadata, render: render, pluginUsage: pluginUsage, renderedContents: renderedContents, userID: user.ID,
		},
		slug: slug, icon: icon, markdown: markdown, message: message, tags: tags, links: links,
		groupIDs: groupIDs, properties: properties, user: user,
	}, nil
}

// executePageSaveTransaction performs the shared ordered page write inside an existing transaction.
func executePageSaveTransaction(ctx context.Context, tx pgx.Tx, mutation pageSaveTransaction) error {
	id, err := savePageRecord(ctx, tx, mutation.record)
	if err != nil {
		return err
	}
	if err := savePageIcon(ctx, tx, mutation.slug, mutation.icon); err != nil {
		return err
	}
	if err := appendPageRevision(ctx, tx, id, mutation.markdown, mutation.message, mutation.user.ID); err != nil {
		return err
	}
	if err := supersedePageReviews(ctx, tx, id); err != nil {
		return err
	}
	if err := replacePageTags(ctx, tx, id, mutation.tags); err != nil {
		return err
	}
	if err := replacePageGroups(ctx, tx, id, mutation.groupIDs, mutation.user); err != nil {
		return err
	}
	if err := replacePageProperties(ctx, tx, id, mutation.properties); err != nil {
		return err
	}
	return replacePageLinks(ctx, tx, id, mutation.links)
}

// preparePageSave validates page metadata and encodes derived JSON values for persistence.
func preparePageSave(
	metadata domain.PageMetadata,
	render domain.PageRender,
) (domain.PageMetadata, any, json.RawMessage, domain.PageRender, error) {
	if !domain.ValidPageStatus(metadata.Status) {
		return domain.PageMetadata{}, nil, nil, domain.PageRender{}, domain.NewValidationError("status", "Choose a valid page status.")
	}
	if metadata.ReviewIntervalDays < 0 {
		return domain.PageMetadata{}, nil, nil, domain.PageRender{}, domain.NewValidationError("review_interval_days", "Choose a valid review interval.")
	}

	metadata.DeprecatedTarget = strings.TrimSpace(metadata.DeprecatedTarget)

	pluginUsage, renderedContents, render, err := preparePageDerivedData(metadata.PluginUsage, render)
	if err != nil {
		return domain.PageMetadata{}, nil, nil, domain.PageRender{}, err
	}

	return metadata, pluginUsage, renderedContents, render, nil
}

// preparePageDerivedData encodes plugin usage and reusable render metadata for a page write.
func preparePageDerivedData(
	usage *pluginusage.Index,
	render domain.PageRender,
) (any, json.RawMessage, domain.PageRender, error) {
	var pluginUsage any
	if usage != nil {
		encoded, err := json.Marshal(usage)
		if err != nil {
			return nil, nil, domain.PageRender{}, fmt.Errorf("encode page plugin usage: %w", err)
		}
		pluginUsage = json.RawMessage(encoded)
	}

	renderedContents, err := json.Marshal(render.Contents)
	if err != nil {
		return nil, nil, domain.PageRender{}, fmt.Errorf("encode rendered page contents: %w", err)
	}
	if render.Fingerprint == "" {
		render.HTML = ""
		renderedContents = []byte("[]")
	}

	return pluginUsage, json.RawMessage(renderedContents), render, nil
}

// savePageRecord creates or updates the pages row and records aliases for renames.
func savePageRecord(ctx context.Context, tx pgx.Tx, record pageSaveRecord) (int64, error) {
	lookupSlug := pageRecordLookupSlug(record)
	id, deleted, found, err := loadPageRecordForUpdate(ctx, tx, lookupSlug)
	if err != nil {
		return 0, err
	}
	if !found {
		return insertPageRecord(ctx, tx, record)
	}
	if deleted {
		return 0, domain.ErrPageInBin
	}
	if err := renamePageRecord(ctx, tx, id, lookupSlug, record.slug); err != nil {
		return 0, err
	}
	if err := updatePageRecord(ctx, tx, id, record); err != nil {
		return 0, err
	}

	return id, nil
}

// pageRecordLookupSlug selects the existing path used to lock an edit or the destination path for a create.
func pageRecordLookupSlug(record pageSaveRecord) string {
	if previous := strings.TrimSpace(record.previousSlug); previous != "" {
		return previous
	}
	return record.slug
}

// loadPageRecordForUpdate locks an existing page row and reports whether it is deleted.
func loadPageRecordForUpdate(ctx context.Context, tx pgx.Tx, slug string) (id int64, deleted, found bool, err error) {
	err = tx.QueryRow(ctx, `
SELECT id,deleted_at IS NOT NULL
FROM pages
WHERE slug=$1
FOR UPDATE`, slug).Scan(&id, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, err
	}

	return id, deleted, true, nil
}

// insertPageRecord creates one page after rejecting aliases that already own the destination path.
func insertPageRecord(ctx context.Context, tx pgx.Tx, record pageSaveRecord) (int64, error) {
	var aliasExists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM page_aliases
  WHERE alias=$1
)`, record.slug).Scan(&aliasExists); err != nil {
		return 0, err
	}
	if aliasExists {
		return 0, domain.ErrAlreadyExists
	}

	var id int64
	err := tx.QueryRow(ctx, `
INSERT INTO pages(
  slug,title,content_language,markdown_content,created_by,updated_by,status,owner_group_id,last_reviewed_at,review_interval_days,deprecated_target,plugin_usage,
  rendered_html,rendered_contents,render_fingerprint,rendered_at
) VALUES(
  $1,$2,$3,$4,$5,$5,$6,NULLIF($7,0),CASE WHEN $8 THEN now() ELSE NULL END,$9,$10,$11::jsonb,
  $12,$13::jsonb,$14,CASE WHEN $14<>'' THEN now() ELSE NULL END
)
RETURNING id`,
		record.slug,
		record.title,
		record.language,
		record.markdown,
		record.userID,
		record.metadata.Status,
		record.metadata.OwnerGroupID,
		record.metadata.MarkReviewed,
		record.metadata.ReviewIntervalDays,
		record.metadata.DeprecatedTarget,
		record.pluginUsage,
		record.render.HTML,
		record.renderedContents,
		record.render.Fingerprint,
	).Scan(&id)

	return id, err
}

// updatePageRecord replaces the mutable columns of one existing page row.
func updatePageRecord(ctx context.Context, tx pgx.Tx, id int64, record pageSaveRecord) error {
	_, err := tx.Exec(ctx, `
UPDATE pages
SET title=$2,content_language=$3,markdown_content=$4,updated_by=$5,updated_at=now(),
    status=$6,owner_group_id=NULLIF($7,0),
    last_reviewed_at=CASE WHEN $8 THEN now() ELSE last_reviewed_at END,
    review_interval_days=$9,deprecated_target=$10,plugin_usage=$11::jsonb,
    rendered_html=$12,rendered_contents=$13::jsonb,render_fingerprint=$14,
    rendered_at=CASE WHEN $14<>'' THEN now() ELSE NULL END
WHERE id=$1`,
		id,
		record.title,
		record.language,
		record.markdown,
		record.userID,
		record.metadata.Status,
		record.metadata.OwnerGroupID,
		record.metadata.MarkReviewed,
		record.metadata.ReviewIntervalDays,
		record.metadata.DeprecatedTarget,
		record.pluginUsage,
		record.render.HTML,
		record.renderedContents,
		record.render.Fingerprint,
	)

	return err
}

// renamePageRecord changes a page slug and preserves the previous slug as an alias.
func renamePageRecord(ctx context.Context, tx pgx.Tx, id int64, oldSlug, newSlug string) error {
	if oldSlug == newSlug {
		return nil
	}

	if err := validatePageDestination(ctx, tx, id, newSlug); err != nil {
		return err
	}
	if err := removeCurrentSlugAlias(ctx, tx, id, newSlug); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
UPDATE pages
SET slug=$2
WHERE id=$1`, id, newSlug); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE navigation_icons
SET path=$2
WHERE path=$1`, oldSlug, newSlug); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
INSERT INTO page_aliases(alias,page_id)
VALUES($1,$2)
ON CONFLICT(alias) DO UPDATE
SET page_id=EXCLUDED.page_id`, oldSlug, id)
	return err
}

// savePageIcon replaces or removes the navigation icon stored for a page path.
func savePageIcon(ctx context.Context, tx pgx.Tx, slug, icon string) error {
	icon = strings.TrimSpace(icon)
	if icon == "" {
		_, err := tx.Exec(ctx, `
DELETE FROM navigation_icons
WHERE path=$1`, slug)
		return err
	}

	_, err := tx.Exec(ctx, `
INSERT INTO navigation_icons(path,icon)
VALUES($1,$2)
ON CONFLICT(path) DO UPDATE SET icon=EXCLUDED.icon`, slug, icon)
	return err
}

// appendPageRevision appends the next immutable revision for a saved page.
func appendPageRevision(ctx context.Context, tx pgx.Tx, pageID int64, markdown, message string, userID int64) error {
	var revision int
	if err := tx.QueryRow(ctx, `
SELECT coalesce(max(revision_number),0)+1
FROM page_revisions
WHERE page_id=$1`, pageID).Scan(&revision); err != nil {
		return err
	}

	_, err := tx.Exec(ctx, `
INSERT INTO page_revisions(page_id,revision_number,markdown_content,created_by,message)
VALUES($1,$2,$3,$4,$5)`, pageID, revision, markdown, userID, message)
	return err
}

// supersedePageReviews closes review requests invalidated by a new page revision.
func supersedePageReviews(ctx context.Context, tx pgx.Tx, pageID int64) error {
	_, err := tx.Exec(ctx, `
UPDATE page_review_requests
SET status='superseded',updated_at=now()
WHERE page_id=$1 AND status IN ('pending','changes_requested')`, pageID)
	return err
}

// replacePageTags replaces all tags assigned to a page.
func replacePageTags(ctx context.Context, tx pgx.Tx, pageID int64, tags []string) error {
	if _, err := tx.Exec(ctx, `
DELETE FROM page_tags
WHERE page_id=$1`, pageID); err != nil {
		return err
	}

	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag == "" {
			continue
		}

		var tagID int64
		if err := tx.QueryRow(ctx, `
INSERT INTO tags(name)
VALUES($1)
ON CONFLICT(name) DO UPDATE
SET name=EXCLUDED.name
RETURNING id`, tag).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO page_tags(page_id,tag_id)
VALUES($1,$2)
ON CONFLICT DO NOTHING`, pageID, tagID); err != nil {
			return err
		}
	}

	return nil
}

// replacePageLinks replaces the normalized outgoing wiki-link targets for a page.
func replacePageLinks(ctx context.Context, tx pgx.Tx, pageID int64, links []string) error {
	if _, err := tx.Exec(ctx, `
DELETE FROM page_links
WHERE source_page_id=$1`, pageID); err != nil {
		return err
	}

	for _, link := range links {
		if _, err := tx.Exec(ctx, `
INSERT INTO page_links(source_page_id,target_slug)
VALUES($1,$2)
ON CONFLICT DO NOTHING`, pageID, link); err != nil {
			return err
		}
	}

	return nil
}
