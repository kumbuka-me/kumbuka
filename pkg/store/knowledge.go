package store

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageProperties returns structured properties for one page.
func (s *Store) PageProperties(ctx context.Context, pageID int64) ([]domain.PageProperty, error) {
	rows, err := s.pool.Query(ctx, `
SELECT key,value
FROM page_properties
WHERE page_id=$1
ORDER BY lower(key),key`, pageID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var properties []domain.PageProperty

	for rows.Next() {
		var item domain.PageProperty
		if err := rows.Scan(&item.Key, &item.Value); err != nil {
			return nil, err
		}

		properties = append(properties, item)
	}

	return properties, rows.Err()
}

// replacePageProperties atomically replaces the structured properties for a page.
func replacePageProperties(ctx context.Context, tx pgx.Tx, pageID int64, properties map[string]string) error {
	if _, err := tx.Exec(ctx, `
DELETE FROM page_properties
WHERE page_id=$1`, pageID); err != nil {
		return err
	}

	keys := slices.Sorted(maps.Keys(properties))

	for _, key := range keys {
		value := strings.TrimSpace(properties[key])
		key = strings.TrimSpace(key)
		if key == "" || value == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO page_properties(page_id,key,value)
VALUES($1,$2,$3)`, pageID, key, value); err != nil {
			return err
		}
	}

	return nil
}

// SavedSearches returns a user's named searches.
func (s *Store) SavedSearches(ctx context.Context, userID int64) ([]domain.SavedSearch, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id,name,query,pinned
FROM saved_searches
WHERE user_id=$1
ORDER BY pinned DESC,lower(name),id`, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var items []domain.SavedSearch

	for rows.Next() {
		var item domain.SavedSearch
		if err := rows.Scan(&item.ID, &item.Name, &item.Query, &item.Pinned); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// SaveSavedSearch creates or updates a named search.
func (s *Store) SaveSavedSearch(ctx context.Context, userID, id int64, name, query string, pinned bool) error {
	name = strings.TrimSpace(name)
	query = strings.TrimSpace(query)
	if name == "" {
		return domain.NewValidationError("name", "A saved search name is required.")
	}
	if query == "" {
		return domain.NewValidationError("query", "A search query is required.")
	}
	if id == 0 {
		_, err := s.pool.Exec(ctx, `
INSERT INTO saved_searches(user_id,name,query,pinned)
VALUES($1,$2,$3,$4)`, userID, name, query, pinned)

		return mutationError(err)
	}

	tag, err := s.pool.Exec(ctx, `
UPDATE saved_searches
SET name=$3,query=$4,pinned=$5
WHERE id=$1 AND user_id=$2`, id, userID, name, query, pinned)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return mutationError(err)
}

// DeleteSavedSearch deletes one named search owned by a user.
func (s *Store) DeleteSavedSearch(ctx context.Context, userID, id int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM saved_searches
WHERE id=$1 AND user_id=$2`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// PageComments returns comments for a page, unresolved first.
func (s *Store) PageComments(ctx context.Context, slug string) ([]domain.PageComment, error) {
	rows, err := s.pool.Query(ctx, `
SELECT c.id,c.page_id,coalesce(u.display_name,u.username,'Deleted user'),c.anchor,c.body,c.resolved_at,c.created_at
FROM page_comments c
JOIN pages p ON p.id=c.page_id
LEFT JOIN users u ON u.id=c.user_id
WHERE p.slug=$1 AND p.deleted_at IS NULL
ORDER BY (c.resolved_at IS NOT NULL),c.created_at`, slug)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var items []domain.PageComment

	for rows.Next() {
		var item domain.PageComment
		if err := rows.Scan(&item.ID, &item.PageID, &item.Author, &item.Anchor, &item.Body, &item.Resolved, &item.CreatedAt); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// AddPageComment adds a comment to a page.
func (s *Store) AddPageComment(ctx context.Context, slug string, userID int64, anchor, body string) (domain.PageComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.PageComment{}, domain.NewValidationError("body", "A comment is required.")
	}

	var item domain.PageComment
	err := s.pool.QueryRow(ctx, `
INSERT INTO page_comments(page_id,user_id,anchor,body)
SELECT id,$2,$3,$4 FROM pages
WHERE slug=$1 AND deleted_at IS NULL
RETURNING id,page_id,$5,anchor,body,resolved_at,created_at`, slug, userID, strings.TrimSpace(anchor), body, "").
		Scan(&item.ID, &item.PageID, &item.Author, &item.Anchor, &item.Body, &item.Resolved, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageComment{}, domain.ErrNotFound
	}

	return item, err
}

// ResolvePageComment resolves or reopens one page comment.
func (s *Store) ResolvePageComment(ctx context.Context, id int64, resolved bool) error {
	var value any

	if resolved {
		value = time.Now()
	}

	tag, err := s.pool.Exec(ctx, `
UPDATE page_comments
SET resolved_at=$2,updated_at=now()
WHERE id=$1`, id, value)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrCommentNotFound
	}

	return err
}

// KnowledgeGraph returns pages and current wiki-link relationships.
func (s *Store) KnowledgeGraph(ctx context.Context, limit int) (domain.KnowledgeGraph, error) {
	if limit <= 0 || limit > 500 {
		limit = 250
	}

	graph := domain.KnowledgeGraph{}
	rows, err := s.pool.Query(ctx, `
SELECT slug,title,status
FROM pages
WHERE deleted_at IS NULL
ORDER BY updated_at DESC
LIMIT $1`, limit)
	if err != nil {
		return graph, err
	}

	allowed := map[string]bool{}

	for rows.Next() {
		var node domain.GraphNode
		if err := rows.Scan(&node.Slug, &node.Title, &node.Status); err != nil {
			rows.Close()
			return graph, err
		}

		graph.Nodes = append(graph.Nodes, node)
		allowed[node.Slug] = true
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return graph, err
	}

	rows.Close()

	rows, err = s.pool.Query(ctx, `
SELECT source.slug,coalesce(target.slug,alias_target.slug,'')
FROM page_links l
JOIN pages source ON source.id=l.source_page_id AND source.deleted_at IS NULL
LEFT JOIN pages target ON target.slug=l.target_slug AND target.deleted_at IS NULL
LEFT JOIN page_aliases a ON a.alias=l.target_slug
LEFT JOIN pages alias_target ON alias_target.id=a.page_id AND alias_target.deleted_at IS NULL
WHERE target.id IS NOT NULL OR alias_target.id IS NOT NULL`)
	if err != nil {
		return graph, err
	}

	defer rows.Close()

	for rows.Next() {
		var edge domain.GraphEdge
		if err := rows.Scan(&edge.Source, &edge.Target); err != nil {
			return graph, err
		}

		if allowed[edge.Source] && allowed[edge.Target] && edge.Source != edge.Target {
			graph.Edges = append(graph.Edges, edge)
		}
	}

	return graph, rows.Err()
}

// RecentEdited returns pages most recently revised by one user.
func (s *Store) RecentEdited(ctx context.Context, userID int64, limit int) ([]domain.RecentEdit, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.id,p.slug,p.title,coalesce(i.icon,''),p.updated_at,coalesce(r.message,'')
FROM pages p
LEFT JOIN navigation_icons i ON i.path=p.slug
LEFT JOIN LATERAL (SELECT message,created_by FROM page_revisions WHERE page_id=p.id ORDER BY revision_number DESC LIMIT 1) r ON true
WHERE p.deleted_at IS NULL AND r.created_by=$1
ORDER BY p.updated_at DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var items []domain.RecentEdit

	for rows.Next() {
		var item domain.RecentEdit
		if err := rows.Scan(&item.ID, &item.Slug, &item.Title, &item.Icon, &item.UpdatedAt, &item.RevisionMessage); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// DueReviewPages returns pages whose configured review interval has elapsed.
func (s *Store) DueReviewPages(ctx context.Context, limit int) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, `
SELECT slug,title,status,review_interval_days,last_reviewed_at
FROM pages
WHERE deleted_at IS NULL AND review_interval_days>0 AND coalesce(last_reviewed_at,created_at)+(review_interval_days || ' days')::interval <= now()
ORDER BY coalesce(last_reviewed_at,created_at),slug
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.Page

	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.Slug, &page.Title, &page.Status, &page.ReviewIntervalDays, &page.LastReviewedAt); err != nil {
			return nil, err
		}

		pages = append(pages, page)
	}

	return pages, rows.Err()
}

// MarkPageReviewed records a documentation review timestamp.
func (s *Store) MarkPageReviewed(ctx context.Context, slug string) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE pages
SET last_reviewed_at=now()
WHERE slug=$1 AND deleted_at IS NULL`, slug)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// movedPage describes one page path included in a tree move.
type movedPage struct {
	// id identifies the persisted page row.
	id int64
	// oldSlug is the canonical path before the move.
	oldSlug string
	// newSlug is the canonical path after the move.
	newSlug string
}

// pageSourceEdit contains a Markdown source rewritten after a page move.
type pageSourceEdit struct {
	// id identifies the page whose Markdown changed.
	id int64
	// markdown contains the rewritten canonical Markdown source.
	markdown string
}

// MovePage moves one page, optionally including descendants, and can refactor direct wiki-link targets.
func (s *Store) MovePage(ctx context.Context, oldSlug, newSlug string, options domain.MovePageOptions, user domain.User) error {
	oldSlug, newSlug, err := normalizeMoveSlugs(oldSlug, newSlug, options)
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	moved, err := loadMovedPages(ctx, tx, oldSlug, newSlug, options.MoveChildren)
	if err != nil {
		return mutationError(err)
	}
	if len(moved) == 0 {
		return domain.ErrNotFound
	}

	if err := validateMoveDestinations(ctx, tx, moved); err != nil {
		return mutationError(err)
	}
	if err := applyPageMoves(ctx, tx, moved, options.KeepAliases, user.ID); err != nil {
		return mutationError(err)
	}
	if err := retargetMovedPageLinks(ctx, tx, moved); err != nil {
		return mutationError(err)
	}
	if options.UpdateIncomingLinks {
		if err := rewriteIncomingWikiLinks(ctx, tx, moved, user.ID); err != nil {
			return mutationError(err)
		}
	}

	return mutationError(tx.Commit(ctx))
}

// normalizeMoveSlugs normalizes and validates the source and destination paths for a move.
func normalizeMoveSlugs(oldSlug, newSlug string, options domain.MovePageOptions) (string, string, error) {
	oldSlug = strings.Trim(strings.TrimSpace(oldSlug), "/")
	newSlug = strings.Trim(strings.TrimSpace(newSlug), "/")
	if oldSlug == "" || newSlug == "" || oldSlug == newSlug {
		return "", "", domain.NewValidationError("slug", "Choose a different, non-empty destination path.")
	}
	if options.MoveChildren && strings.HasPrefix(newSlug, oldSlug+"/") {
		return "", "", domain.NewValidationError("slug", "A page tree cannot be moved inside itself.")
	}

	return oldSlug, newSlug, nil
}

// loadMovedPages loads the source page set and calculates each destination path.
func loadMovedPages(ctx context.Context, tx pgx.Tx, oldSlug, newSlug string, moveChildren bool) ([]movedPage, error) {
	query := `
SELECT id,slug
FROM pages
WHERE deleted_at IS NULL AND slug=$1
ORDER BY length(slug),slug`
	if moveChildren {
		query = `
SELECT id,slug
FROM pages
WHERE deleted_at IS NULL AND (slug=$1 OR slug LIKE $1 || '/%')
ORDER BY length(slug),slug`
	}

	rows, err := tx.Query(ctx, query, oldSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var moved []movedPage
	for rows.Next() {
		var item movedPage
		if err := rows.Scan(&item.id, &item.oldSlug); err != nil {
			return nil, err
		}

		item.newSlug = newSlug + strings.TrimPrefix(item.oldSlug, oldSlug)
		moved = append(moved, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return moved, nil
}

// validateMoveDestinations ensures no destination collides with pages or aliases outside the move set.
func validateMoveDestinations(ctx context.Context, tx pgx.Tx, moved []movedPage) error {
	movingIDs := make([]int64, 0, len(moved))
	for _, item := range moved {
		movingIDs = append(movingIDs, item.id)
	}

	for _, item := range moved {
		var conflict bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM pages WHERE slug=$1 AND id<>ALL($2::bigint[])) OR EXISTS(SELECT 1 FROM page_aliases WHERE alias=$1 AND page_id<>ALL($2::bigint[]))`, item.newSlug, movingIDs).Scan(&conflict); err != nil {
			return err
		}
		if conflict {
			return domain.ErrAlreadyExists
		}
	}

	return nil
}

// applyPageMoves updates page paths, navigation icons, and optional aliases deepest-first.
func applyPageMoves(ctx context.Context, tx pgx.Tx, moved []movedPage, keepAliases bool, userID int64) error {
	// Update deepest paths first so unique path constraints never collide with descendants.
	for _, item := range slices.Backward(moved) {
		if _, err := tx.Exec(ctx, `
UPDATE pages
SET slug=$2,updated_by=$3,updated_at=now()
WHERE id=$1`, item.id, item.newSlug, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
UPDATE navigation_icons
SET path=$2
WHERE path=$1`, item.oldSlug, item.newSlug); err != nil {
			return err
		}
		if !keepAliases {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO page_aliases(alias,page_id)
VALUES($1,$2)
ON CONFLICT(alias) DO UPDATE
SET page_id=EXCLUDED.page_id`, item.oldSlug, item.id); err != nil {
			return err
		}
	}

	return nil
}

// retargetMovedPageLinks updates normalized link targets that point at moved pages.
func retargetMovedPageLinks(ctx context.Context, tx pgx.Tx, moved []movedPage) error {
	for _, item := range moved {
		if _, err := tx.Exec(ctx, `
UPDATE page_links
SET target_slug=$2
WHERE target_slug=$1`, item.oldSlug, item.newSlug); err != nil {
			return err
		}
	}

	return nil
}

// rewriteIncomingWikiLinks updates direct wiki-link source text and records a revision for each changed page.
func rewriteIncomingWikiLinks(ctx context.Context, tx pgx.Tx, moved []movedPage, userID int64) error {
	rows, err := tx.Query(ctx, `
SELECT id,markdown_content
FROM pages
WHERE deleted_at IS NULL AND markdown_content LIKE '%[[%'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var edits []pageSourceEdit
	for rows.Next() {
		var item pageSourceEdit
		if err := rows.Scan(&item.id, &item.markdown); err != nil {
			return err
		}

		updated := item.markdown
		for _, page := range moved {
			updated = rewriteDirectWikiTarget(updated, page.oldSlug, page.newSlug)
		}
		if updated != item.markdown {
			item.markdown = updated
			edits = append(edits, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, edit := range edits {
		if _, err := tx.Exec(ctx, `
UPDATE pages
SET markdown_content=$2,plugin_usage=NULL,updated_by=$3,updated_at=now()
WHERE id=$1`, edit.id, edit.markdown, userID); err != nil {
			return err
		}
		if err := appendPageRevision(ctx, tx, edit.id, edit.markdown, "Update links after page move", userID); err != nil {
			return err
		}
	}

	return nil
}

// rewriteDirectWikiTarget updates direct wiki links while preserving their labels.
func rewriteDirectWikiTarget(source, oldSlug, newSlug string) string {
	source = strings.ReplaceAll(source, "[["+oldSlug+"]]", "[["+newSlug+"]]")
	source = strings.ReplaceAll(source, "[["+oldSlug+"|", "[["+newSlug+"|")
	source = strings.ReplaceAll(source, "[["+oldSlug+"#", "[["+newSlug+"#")

	return source
}

// PageInventory returns pages with lifecycle metadata for administration.
func (s *Store) PageInventory(ctx context.Context) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.id,p.slug,p.title,p.status,coalesce(p.owner_group_id,0),coalesce(g.name,''),p.last_reviewed_at,p.review_interval_days,p.updated_at
FROM pages p
LEFT JOIN wiki_groups g ON g.id=p.owner_group_id
WHERE p.deleted_at IS NULL
ORDER BY p.slug`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pages []domain.Page

	for rows.Next() {
		var page domain.Page
		if err := rows.Scan(&page.ID, &page.Slug, &page.Title, &page.Status, &page.OwnerGroupID, &page.OwnerGroup, &page.LastReviewedAt, &page.ReviewIntervalDays, &page.UpdatedAt); err != nil {
			return nil, err
		}

		pages = append(pages, page)
	}

	return pages, rows.Err()
}

// BulkSetPageStatus updates lifecycle status for selected pages.
func (s *Store) BulkSetPageStatus(ctx context.Context, slugs []string, status string) error {
	if !domain.ValidPageStatus(status) {
		return domain.NewValidationError("status", "Choose a valid page status.")
	}

	_, err := s.pool.Exec(ctx, `
UPDATE pages
SET status=$2,updated_at=now()
WHERE slug=ANY($1::text[]) AND deleted_at IS NULL`, slugs, status)

	return err
}

// BulkAddPageTag adds one normalized tag to selected pages.
func (s *Store) BulkAddPageTag(ctx context.Context, slugs []string, tag string) error {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return domain.NewValidationError("tag", "A tag is required.")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

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
SELECT id,$2 FROM pages
WHERE slug=ANY($1::text[]) AND deleted_at IS NULL
ON CONFLICT DO NOTHING`, slugs, tagID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// BulkAssignPageGroup assigns one collaboration group to selected pages.
func (s *Store) BulkAssignPageGroup(ctx context.Context, slugs []string, groupID int64) error {
	if groupID <= 0 {
		return domain.NewValidationError("group_id", "Choose a valid group.")
	}

	_, err := s.pool.Exec(ctx, `
INSERT INTO page_groups(page_id,group_id)
SELECT id,$2 FROM pages
WHERE slug=ANY($1::text[]) AND deleted_at IS NULL
ON CONFLICT DO NOTHING`, slugs, groupID)

	return mutationError(err)
}

// BulkDeletePages moves selected pages to the recycle bin.
func (s *Store) BulkDeletePages(ctx context.Context, slugs []string, userID int64) error {
	_, err := s.pool.Exec(ctx, `
UPDATE pages
SET deleted_at=now(),deleted_by=$2
WHERE slug=ANY($1::text[]) AND deleted_at IS NULL`, slugs, userID)
	return err
}

// PageAliases returns persisted redirect aliases and their current target slugs.
func (s *Store) PageAliases(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT a.alias,p.slug
FROM page_aliases a
JOIN pages p ON p.id=a.page_id
WHERE p.deleted_at IS NULL
ORDER BY a.alias`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	aliases := map[string]string{}

	for rows.Next() {
		var alias, target string
		if err := rows.Scan(&alias, &target); err != nil {
			return nil, err
		}

		aliases[alias] = target
	}

	return aliases, rows.Err()
}
