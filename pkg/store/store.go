package store

import (
	"cmp"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Store provides PostgreSQL-backed persistence for Kumbuka data.
type Store struct {
	// pool is the PostgreSQL connection pool used by store operations.
	pool *pgxpool.Pool
}

// Open connects to PostgreSQL and applies pending embedded migrations.
func Open(ctx context.Context, url string, logger *slog.Logger) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}

	s := &Store{pool: pool}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	if err := s.migrate(ctx, logger); err != nil {
		pool.Close()
		return nil, err
	}

	return s, nil
}

// Close releases the PostgreSQL connection pool.
func (s *Store) Close() { s.pool.Close() }

// Ping verifies that PostgreSQL is reachable.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// migrate applies unapplied embedded SQL migrations in version order.
func (s *Store) migrate(ctx context.Context, logger *slog.Logger) error {
	// Serialize schema initialization and migration discovery across app instances.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(734627198236)`); err != nil {
		return err
	}
	var applied []fs.DirEntry

	// Keep migration history in the same database so startup can safely skip
	// schema changes that have already been committed.
	if _, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}

	// Sort embedded entries by their numeric filename prefix because lexical
	// filename order would place migration 10 before migration 2.
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}

	slices.SortFunc(entries, compareMigrationEntries)

	for _, e := range entries {
		if !isSQLFile(e) {
			continue
		}

		// Extract the migration version number from the filename.
		v, err := migrationVersion(e.Name())
		if err != nil {
			return fmt.Errorf("invalid migration %s", e.Name())
		}

		var exists bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, v).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}

		sql, err := migrationFiles.ReadFile("migrations/" + e.Name())
		if err != nil {
			return err
		}

		// Apply the schema change and record its version atomically. A failed
		// statement therefore remains eligible for retry on the next startup.
		if _, err = tx.Exec(ctx, string(sql)); err == nil {
			_, err = tx.Exec(ctx, `
INSERT INTO schema_migrations(version)
VALUES($1)`, v)
		}

		if err != nil {
			return fmt.Errorf("migration %d: %w", v, err)
		}
		applied = append(applied, e)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	for _, e := range applied {
		v, _ := migrationVersion(e.Name())
		logger.Info(
			"applied database migration",
			"event", "database_migration_applied",
			"version", v,
			"migration", e.Name(),
		)
	}

	return nil
}

// migrationVersion parses the numeric prefix of an embedded migration filename.
func migrationVersion(name string) (version int, err error) {
	prefix, _, _ := strings.Cut(name, "_")
	return strconv.Atoi(prefix)
}

// compareMigrationEntries orders migrations by their numeric filename prefix.
func compareMigrationEntries(left, right fs.DirEntry) int {
	leftVersion, _ := migrationVersion(left.Name())
	rightVersion, _ := migrationVersion(right.Name())

	return cmp.Compare(leftVersion, rightVersion)
}

// isSQLFile reports whether e is a regular SQL migration file.
func isSQLFile(e fs.DirEntry) bool {
	return !e.IsDir() && strings.HasSuffix(e.Name(), ".sql")
}

// EnsureAdministrator creates or refreshes the administrator used by no-auth mode.
func (s *Store) EnsureAdministrator(ctx context.Context, username, email, displayName string) (domain.User, error) {
	displayName = cmp.Or(displayName, username)

	var user domain.User
	err := s.pool.QueryRow(ctx, `
INSERT INTO users(username,email,display_name,role,last_login)
VALUES($1,$2,$3,'admin',now())
ON CONFLICT(username) DO UPDATE SET
  email=EXCLUDED.email,
  display_name=EXCLUDED.display_name,
  role='admin',
  enabled=true,
  last_login=now()
RETURNING id,username,email,display_name,role,enabled,session_version`, username, email, displayName).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.Enabled,
		&user.SessionVersion,
	)

	return user, err
}

// TrustedProxyUser refreshes a trusted-proxy user by username or creates one when registration is enabled.
func (s *Store) TrustedProxyUser(ctx context.Context, username, email, displayName string) (domain.User, error) {
	displayName = cmp.Or(displayName, username)

	var user domain.User
	err := s.pool.QueryRow(ctx, `
UPDATE users
SET email=$2,
    display_name=$3,
    last_login=CASE WHEN enabled THEN now() ELSE last_login END
WHERE username=$1
RETURNING id,username,email,display_name,role,enabled,session_version`, username, email, displayName).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}

	settings, err := s.ApplicationSettings(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if !settings.AllowUserRegistration {
		return domain.User{}, domain.ErrRegistrationDisabled
	}

	err = s.pool.QueryRow(ctx, `
INSERT INTO users(username,email,display_name,last_login)
VALUES($1,$2,$3,now())
ON CONFLICT(username) DO UPDATE
SET email=EXCLUDED.email,display_name=EXCLUDED.display_name,last_login=now()
RETURNING id,username,email,display_name,role,enabled,session_version`, username, email, displayName).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion)

	return user, err
}

// UserByToken authenticates an API bearer token and updates its last-used timestamp.
func (s *Store) UserByToken(ctx context.Context, token string) (domain.User, error) {
	h := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(h[:])
	var u domain.User
	err := s.pool.QueryRow(ctx, `
UPDATE api_tokens t
SET last_used=now() FROM users u
WHERE t.token_hash=$1 AND coalesce(t.user_id,t.created_by)=u.id AND u.enabled AND (t.expires_at IS NULL OR t.expires_at>now())
RETURNING u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version`, hash).
		Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Enabled, &u.SessionVersion)

	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	}

	return u, err
}

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

// SavePageRender replaces the reusable render artifact when the page has not
// changed since it was read. A concurrent edit simply makes this refresh a no-op.
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
	rows, err := s.pool.Query(
		ctx,
		pageSelect+`
WHERE p.deleted_at IS NULL
GROUP BY p.id,u.id
ORDER BY p.updated_at DESC
LIMIT $1`,
		limit,
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
	metadata, pluginUsage, renderedContents, render, err := preparePageSave(metadata, render)
	if err != nil {
		return domain.Page{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Page{}, mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := validateAssignableGroup(ctx, tx, metadata.OwnerGroupID, user); err != nil {
		return domain.Page{}, mutationError(err)
	}

	id, err := savePageRecord(ctx, tx, pageSaveRecord{
		previousSlug:     previousSlug,
		slug:             slug,
		title:            title,
		language:         language,
		markdown:         markdown,
		metadata:         metadata,
		render:           render,
		pluginUsage:      pluginUsage,
		renderedContents: renderedContents,
		userID:           user.ID,
	})
	if err != nil {
		return domain.Page{}, mutationError(err)
	}

	if err := savePageIcon(ctx, tx, slug, icon); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := appendPageRevision(ctx, tx, id, markdown, message, user.ID); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := supersedePageReviews(ctx, tx, id); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageTags(ctx, tx, id, tags); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageGroups(ctx, tx, id, groupIDs, user); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageProperties(ctx, tx, id, properties); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageLinks(ctx, tx, id, links); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Page{}, mutationError(err)
	}

	return s.GetPage(ctx, slug)
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

	var pluginUsage any
	if metadata.PluginUsage != nil {
		encoded, err := json.Marshal(metadata.PluginUsage)
		if err != nil {
			return domain.PageMetadata{}, nil, nil, domain.PageRender{}, fmt.Errorf("encode page plugin usage: %w", err)
		}
		pluginUsage = json.RawMessage(encoded)
	}

	renderedContents, err := json.Marshal(render.Contents)
	if err != nil {
		return domain.PageMetadata{}, nil, nil, domain.PageRender{}, fmt.Errorf("encode rendered page contents: %w", err)
	}
	if render.Fingerprint == "" {
		render.HTML = ""
		renderedContents = []byte("[]")
	}

	return metadata, pluginUsage, json.RawMessage(renderedContents), render, nil
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

// Tags returns all known tags as JSON.
func (s *Store) Tags(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT name
FROM tags
ORDER BY name`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var out []string

	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}

		out = append(out, v)
	}

	return out, rows.Err()
}
