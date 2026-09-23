package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

const pageSelect = `
SELECT p.id,p.slug,p.title,coalesce(max(ni.icon),''),p.markdown_content,coalesce(p.created_by,0),coalesce(p.updated_by,0),coalesce(u.display_name,u.username,''),p.created_at,p.updated_at,p.view_count,coalesce(array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),'{}'),p.status,p.plugin_usage
FROM pages p
LEFT JOIN navigation_icons ni ON ni.path=p.slug
LEFT JOIN users u ON u.id=p.updated_by
LEFT JOIN page_tags pt ON pt.page_id=p.id
LEFT JOIN tags t ON t.id=pt.tag_id`

// scanPage scans the common page projection and normalizes missing rows.
func scanPage(row pgx.Row) (domain.Page, error) {
	var p domain.Page
	var pluginUsage json.RawMessage
	err := row.Scan(
		&p.ID,
		&p.Slug,
		&p.Title,
		&p.Icon,
		&p.Markdown,
		&p.CreatedBy,
		&p.UpdatedBy,
		&p.Author,
		&p.CreatedAt,
		&p.UpdatedAt,
		&p.ViewCount,
		&p.Tags,
		&p.Status,
		&pluginUsage,
	)

	if err == nil && len(pluginUsage) != 0 {
		var usage pluginusage.Index
		if json.Unmarshal(pluginUsage, &usage) == nil {
			p.PluginUsage = &usage
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	}

	return p, err
}

// GetPage returns a page by slug.
func (s *Store) GetPage(ctx context.Context, slug string) (domain.Page, error) {
	page, err := scanPage(
		s.pool.QueryRow(ctx, pageSelect+`
WHERE p.slug=$1 AND p.deleted_at IS NULL
GROUP BY p.id,u.id`, slug),
	)
	if err != nil {
		return domain.Page{}, err
	}

	page.Groups, err = s.PageGroups(ctx, page.ID)
	if err != nil {
		return domain.Page{}, err
	}
	var renderedContents json.RawMessage
	if err := s.pool.QueryRow(ctx, `
SELECT p.content_language,p.status,coalesce(p.owner_group_id,0),coalesce(g.name,''),p.last_reviewed_at,p.review_interval_days,p.deprecated_target,
       p.rendered_html,p.rendered_contents,p.render_fingerprint
FROM pages p
LEFT JOIN wiki_groups g ON g.id=p.owner_group_id
WHERE p.id=$1`, page.ID).Scan(
		&page.Language,
		&page.Status,
		&page.OwnerGroupID,
		&page.OwnerGroup,
		&page.LastReviewedAt,
		&page.ReviewIntervalDays,
		&page.DeprecatedTarget,
		&page.Render.HTML,
		&renderedContents,
		&page.Render.Fingerprint,
	); err != nil {
		return domain.Page{}, err
	}

	if len(renderedContents) != 0 {
		if err := json.Unmarshal(renderedContents, &page.Render.Contents); err != nil {
			// Render artifacts are derived data. Corrupt metadata must fall back to
			// the canonical Markdown instead of making the page unavailable.
			page.Render = domain.PageRender{}
		}
	}

	page.Properties, err = s.PageProperties(ctx, page.ID)
	if err != nil {
		return domain.Page{}, err
	}

	return page, nil
}

// SavePageRender replaces the reusable render artifact when the page has not changed since it was read. A concurrent edit simply makes this refresh a no-op.
func (s *Store) SavePageRender(ctx context.Context, pageID int64, updatedAt time.Time, render domain.PageRender) error {
	contents, err := json.Marshal(render.Contents)
	if err != nil {
		return fmt.Errorf("encode rendered page contents: %w", err)
	}
	if render.Fingerprint == "" {
		render.HTML = ""
		contents = []byte("[]")
	}
	_, err = s.pool.Exec(ctx, `
UPDATE pages
SET rendered_html=$3,rendered_contents=$4::jsonb,render_fingerprint=$5,
    rendered_at=CASE WHEN $5<>'' THEN now() ELSE NULL END
WHERE id=$1 AND updated_at=$2`, pageID, updatedAt, render.HTML, json.RawMessage(contents), render.Fingerprint)
	return mutationError(err)
}

// ListPages returns recently updated pages up to the requested limit.
func (s *Store) ListPages(ctx context.Context, limit int) ([]domain.Page, error) {
	return s.ListPagesPage(ctx, limit, 0)
}

// ListPagesPage returns one deterministic window of recently updated pages.
func (s *Store) ListPagesPage(ctx context.Context, limit, offset int) ([]domain.Page, error) {
	rows, err := s.pool.Query(
		ctx,
		pageSelect+`
WHERE p.deleted_at IS NULL
GROUP BY p.id,u.id
ORDER BY p.updated_at DESC,p.id DESC
LIMIT $1 OFFSET $2`,
		limit,
		offset,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return collectPages(rows)
}

// NavigationPages returns the minimal page data required to build navigation.
func (s *Store) NavigationPages(ctx context.Context) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.slug,p.title,coalesce(i.icon,'')
FROM pages p
LEFT JOIN navigation_icons i ON i.path=p.slug
WHERE p.deleted_at IS NULL
ORDER BY p.slug`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.Page

	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.Slug, &page.Title, &page.Icon); err != nil {
			return nil, err
		}

		pages = append(pages, page)
	}

	return pages, rows.Err()
}

// collectPages scans all rows from a common page query.
func collectPages(rows pgx.Rows) ([]domain.Page, error) {
	var out []domain.Page

	for rows.Next() {
		p, e := scanPage(rows)
		if e != nil {
			return nil, e
		}

		out = append(out, p)
	}

	return out, rows.Err()
}

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
	if err := validateAssignableGroup(ctx, tx, mutation.record.metadata.OwnerGroupID, mutation.user); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := executePageSaveTransaction(ctx, tx, mutation); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Page{}, mutationError(err)
	}
	return s.GetPage(ctx, slug)
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
	lookupSlug := strings.TrimSpace(record.previousSlug)
	if lookupSlug == "" {
		lookupSlug = record.slug
	}

	var id int64
	var deleted bool
	err := tx.QueryRow(ctx, `
SELECT id,deleted_at IS NOT NULL
FROM pages
WHERE slug=$1 FOR UPDATE`, lookupSlug).Scan(&id, &deleted)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		var aliasExists bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM page_aliases WHERE alias=$1)`, record.slug).Scan(&aliasExists); err != nil {
			return 0, err
		}
		if aliasExists {
			return 0, domain.ErrAlreadyExists
		}

		err = tx.QueryRow(ctx, `
INSERT INTO pages(
  slug,title,content_language,markdown_content,created_by,updated_by,status,owner_group_id,last_reviewed_at,review_interval_days,deprecated_target,plugin_usage,
  rendered_html,rendered_contents,render_fingerprint,rendered_at
) VALUES(
  $1,$2,$3,$4,$5,$5,$6,NULLIF($7,0),CASE WHEN $8 THEN now() ELSE NULL END,$9,$10,$11::jsonb,
  $12,$13::jsonb,$14,CASE WHEN $14<>'' THEN now() ELSE NULL END
) RETURNING id`,
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
	case err != nil:
		return 0, err
	case deleted:
		return 0, domain.ErrPageInBin
	default:
		if err := renamePageRecord(ctx, tx, id, lookupSlug, record.slug); err != nil {
			return 0, err
		}

		_, err = tx.Exec(ctx, `
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
	}
	if err != nil {
		return 0, err
	}

	return id, nil
}

// renamePageRecord changes a page slug and preserves the previous slug as an alias.
func renamePageRecord(ctx context.Context, tx pgx.Tx, id int64, oldSlug, newSlug string) error {
	if oldSlug == newSlug {
		return nil
	}

	var conflict bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM pages WHERE slug=$1 AND id<>$2) OR EXISTS(SELECT 1 FROM page_aliases WHERE alias=$1 AND page_id<>$2)`, newSlug, id).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return domain.ErrAlreadyExists
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

// DeletePage moves a page into the recycle bin.
func (s *Store) DeletePage(ctx context.Context, slug string, userID int64) error {
	tag, err := s.pool.Exec(
		ctx,
		`
UPDATE pages
SET deleted_at=now(),deleted_by=$2
WHERE slug=$1 AND deleted_at IS NULL`,
		slug,
		userID,
	)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// DeletedPages returns pages currently held in the recycle bin, newest deletion first.
func (s *Store) DeletedPages(ctx context.Context) ([]domain.DeletedPage, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.id,p.slug,p.title,coalesce(max(ni.icon),''),p.markdown_content,coalesce(p.created_by,0),coalesce(p.updated_by,0),
       coalesce(editor.display_name,editor.username,''),p.created_at,p.updated_at,p.view_count,
       coalesce(array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),'{}'),
       p.deleted_at,coalesce(deleter.display_name,deleter.username,'')
FROM pages p
LEFT JOIN navigation_icons ni ON ni.path=p.slug
LEFT JOIN users editor ON editor.id=p.updated_by
LEFT JOIN users deleter ON deleter.id=p.deleted_by
LEFT JOIN page_tags pt ON pt.page_id=p.id
LEFT JOIN tags t ON t.id=pt.tag_id
WHERE p.deleted_at IS NOT NULL
GROUP BY p.id,editor.id,deleter.id
ORDER BY p.deleted_at DESC,p.id DESC`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.DeletedPage

	for rows.Next() {
		var item domain.DeletedPage
		if err := rows.Scan(&item.ID, &item.Slug, &item.Title, &item.Icon, &item.Markdown, &item.CreatedBy, &item.UpdatedBy, &item.Author, &item.CreatedAt, &item.UpdatedAt, &item.ViewCount, &item.Tags, &item.DeletedAt, &item.DeletedBy); err != nil {
			return nil, err
		}

		pages = append(pages, item)
	}

	return pages, rows.Err()
}

// RestorePage restores a page from the recycle bin.
func (s *Store) RestorePage(ctx context.Context, slug string) error {
	tag, err := s.pool.Exec(
		ctx,
		`
UPDATE pages
SET deleted_at=NULL,deleted_by=NULL
WHERE slug=$1 AND deleted_at IS NOT NULL`,
		slug,
	)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return mutationError(err)
}

// PermanentlyDeletePage removes one page already held in the recycle bin.
func (s *Store) PermanentlyDeletePage(ctx context.Context, slug string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
DELETE FROM pages
WHERE slug=$1
  AND deleted_at IS NOT NULL`, slug)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM navigation_icons AS icon
WHERE NOT EXISTS (
  SELECT 1
  FROM pages
  WHERE slug=icon.path
     OR slug LIKE icon.path || '/%'
)`); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// RecordView increments page views and records the user's most recent view.
func (s *Store) RecordView(ctx context.Context, slug string, userID int64) error {
	_, err := s.pool.Exec(
		ctx,
		`
WITH p AS (UPDATE pages SET view_count=view_count+1 WHERE slug=$1 AND deleted_at IS NULL RETURNING id) INSERT INTO page_views(user_id,page_id,viewed_at) SELECT $2,id,now()
FROM p
ON CONFLICT(user_id,page_id) DO UPDATE
SET viewed_at=now()`,
		slug,
		userID,
	)
	return err
}

// SetFavorite adds or removes a page from a user's favorites.
func (s *Store) SetFavorite(ctx context.Context, slug string, userID int64, on bool) error {
	if on {
		_, err := s.pool.Exec(
			ctx,
			`
INSERT INTO favorites(user_id,page_id)
SELECT $2,id FROM pages
WHERE slug=$1 AND deleted_at IS NULL
ON CONFLICT DO NOTHING`,
			slug,
			userID,
		)
		return err
	}

	_, err := s.pool.Exec(
		ctx,
		`
DELETE FROM favorites f
USING pages p
WHERE f.page_id=p.id AND p.slug=$1 AND p.deleted_at IS NULL AND f.user_id=$2`,
		slug,
		userID,
	)

	return err
}

// IsFavorite reports whether a page is currently pinned as a favorite by the user.
func (s *Store) IsFavorite(ctx context.Context, slug string, userID int64) (bool, error) {
	var favorite bool
	err := s.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM favorites f
  JOIN pages p ON p.id=f.page_id
  WHERE f.user_id=$2 AND p.slug=$1 AND p.deleted_at IS NULL
)`, slug, userID).Scan(&favorite)

	return favorite, err
}

// Favorites returns a user's favorite pages in newest-first order.
func (s *Store) Favorites(ctx context.Context, userID int64) ([]domain.Page, error) {
	rows, err := s.pool.Query(
		ctx,
		pageSelect+`
JOIN favorites f ON f.page_id=p.id AND f.user_id=$1
WHERE p.deleted_at IS NULL
GROUP BY p.id,u.id,f.created_at
ORDER BY f.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return collectPages(rows)
}

// Backlinks returns pages that reference the supplied page slug.
func (s *Store) Backlinks(ctx context.Context, slug string) ([]domain.Page, error) {
	base := slug
	if _, segment, ok := strings.CutLast(slug, "/"); ok {
		base = segment
	}

	rows, err := s.pool.Query(
		ctx,
		pageSelect+`
JOIN page_links l ON l.source_page_id=p.id AND l.target_slug IN ($1,$2)
WHERE p.deleted_at IS NULL
GROUP BY p.id,u.id
ORDER BY p.title`,
		slug,
		base,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return collectPages(rows)
}

// ResolvePageAlias resolves a historical page path to its current active slug.
func (s *Store) ResolvePageAlias(ctx context.Context, alias string) (string, error) {
	var slug string
	err := s.pool.QueryRow(ctx, `
SELECT p.slug
FROM page_aliases a
JOIN pages p ON p.id=a.page_id AND p.deleted_at IS NULL
WHERE a.alias=$1`, alias).Scan(&slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}

	return slug, err
}

// LatestRevision returns the newest revision and total count, or a zero count when none exist.
func (s *Store) LatestRevision(ctx context.Context, slug string) (record revision.Revision, count int, err error) {
	const query = `
SELECT
  r.id,
  r.revision_number,
  coalesce(u.display_name,u.username,''),
  r.created_at,
  r.message,
  r.markdown_content,
  coalesce((SELECT previous.markdown_content FROM page_revisions previous WHERE previous.page_id=r.page_id AND previous.revision_number=r.revision_number-1),''),
  count(*) OVER()
FROM page_revisions r
JOIN pages p ON p.id=r.page_id
LEFT JOIN users u ON u.id=r.created_by
WHERE p.slug=$1 AND p.deleted_at IS NULL
ORDER BY r.revision_number DESC
LIMIT 1`

	var markdown, previous string
	err = s.pool.QueryRow(ctx, query, slug).Scan(
		&record.ID,
		&record.Number,
		&record.Author,
		&record.CreatedAt,
		&record.Message,
		&markdown,
		&previous,
		&count,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return revision.Revision{}, 0, nil
	}

	if err == nil {
		record.Markdown = markdown
		record.PreviousMarkdown = previous
	}

	return record, count, err
}

// Revision returns one persisted page revision by revision number.
func (s *Store) Revision(ctx context.Context, slug string, number int) (revision.Revision, error) {
	var record revision.Revision
	err := s.pool.QueryRow(ctx, `
SELECT r.id,r.revision_number,coalesce(u.display_name,u.username,''),r.created_at,r.message,r.markdown_content
FROM page_revisions r
JOIN pages p ON p.id=r.page_id
LEFT JOIN users u ON u.id=r.created_by
WHERE p.slug=$1 AND p.deleted_at IS NULL AND r.revision_number=$2`, slug, number).Scan(
		&record.ID,
		&record.Number,
		&record.Author,
		&record.CreatedAt,
		&record.Message,
		&record.Markdown,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return revision.Revision{}, domain.ErrRevisionNotFound
	}

	return record, err
}

// Revisions returns a page's revision metadata in newest-first order.
func (s *Store) Revisions(ctx context.Context, slug string) ([]revision.Revision, error) {
	const query = `
WITH history AS (
  SELECT
    r.id,
    r.revision_number,
    r.created_by,
    r.created_at,
    r.message,
    r.markdown_content,
    lag(r.markdown_content,1,'') OVER (ORDER BY r.revision_number) AS previous_markdown
  FROM page_revisions r
  JOIN pages p ON p.id=r.page_id
  WHERE p.slug=$1 AND p.deleted_at IS NULL
)
SELECT h.id,h.revision_number,coalesce(u.display_name,u.username,''),h.created_at,h.message,h.markdown_content,h.previous_markdown
FROM history h
LEFT JOIN users u ON u.id=h.created_by
ORDER BY h.revision_number DESC`

	rows, err := s.pool.Query(ctx, query, slug)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var revisions []revision.Revision

	for rows.Next() {
		var record revision.Revision
		if err := rows.Scan(
			&record.ID,
			&record.Number,
			&record.Author,
			&record.CreatedAt,
			&record.Message,
			&record.Markdown,
			&record.PreviousMarkdown,
		); err != nil {
			return nil, err
		}

		revisions = append(revisions, record)
	}

	return revisions, rows.Err()
}

// TaggedPages returns page slugs and tags for access-aware tag discovery.
func (s *Store) TaggedPages(ctx context.Context) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.slug,array_agg(t.name ORDER BY t.name)
FROM pages p
JOIN page_tags pt ON pt.page_id=p.id
JOIN tags t ON t.id=pt.tag_id
WHERE p.deleted_at IS NULL
GROUP BY p.id
ORDER BY p.slug`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.Page
	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.Slug, &page.Tags); err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}

	return pages, rows.Err()
}
