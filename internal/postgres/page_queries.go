package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

const pageSelect = `
SELECT p.id,p.slug,p.title,coalesce(max(ni.icon),''),p.markdown_content,coalesce(p.created_by,0),coalesce(p.updated_by,0),coalesce(u.display_name,u.username,''),p.created_at,p.updated_at,p.view_count,coalesce(array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),'{}'),p.status,p.plugin_usage
FROM pages p
LEFT JOIN navigation_icons ni ON ni.path=p.slug
LEFT JOIN users u ON u.id=p.updated_by
LEFT JOIN page_tags pt ON pt.page_id=p.id
LEFT JOIN tags t ON t.id=pt.tag_id`

const pageSummarySelect = `
SELECT p.id,p.slug,p.title,coalesce(max(ni.icon),''),coalesce(p.created_by,0),coalesce(p.updated_by,0),coalesce(u.display_name,u.username,''),p.created_at,p.updated_at,p.view_count,coalesce(array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL),'{}'),p.status
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

	if err == nil {
		decodePagePluginUsage(&p, pluginUsage)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	}

	return p, err
}

// scanPageSummary scans the body-free projection used by list-style reads.
func scanPageSummary(row pgx.Row) (domain.Page, error) {
	var page domain.Page
	err := row.Scan(
		&page.ID,
		&page.Slug,
		&page.Title,
		&page.Icon,
		&page.CreatedBy,
		&page.UpdatedBy,
		&page.Author,
		&page.CreatedAt,
		&page.UpdatedAt,
		&page.ViewCount,
		&page.Tags,
		&page.Status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	}
	return page, err
}

const completePageSelect = `
SELECT
  p.id,
  p.slug,
  p.title,
  coalesce((SELECT ni.icon FROM navigation_icons ni WHERE ni.path=p.slug LIMIT 1),''),
  p.markdown_content,
  coalesce(p.created_by,0),
  coalesce(p.updated_by,0),
  coalesce(u.display_name,u.username,''),
  p.created_at,
  p.updated_at,
  p.view_count,
  coalesce((
    SELECT array_agg(t.name ORDER BY t.name)
    FROM page_tags pt
    JOIN tags t ON t.id=pt.tag_id
    WHERE pt.page_id=p.id
  ),'{}'),
  p.status,
  p.plugin_usage,
  p.content_language,
  coalesce(p.owner_group_id,0),
  coalesce(owner.name,''),
  p.last_reviewed_at,
  p.review_interval_days,
  p.deprecated_target,
  p.rendered_html,
  p.rendered_contents,
  p.render_fingerprint,
  coalesce((
    SELECT jsonb_agg(
      jsonb_build_object('id',g.id,'name',g.name)
      ORDER BY lower(g.name),g.id
    )
    FROM page_groups pg
    JOIN wiki_groups g ON g.id=pg.group_id
    WHERE pg.page_id=p.id
  ),'[]'::jsonb),
  coalesce((
    SELECT jsonb_agg(
      jsonb_build_object('key',pp.key,'value',pp.value)
      ORDER BY lower(pp.key),pp.key
    )
    FROM page_properties pp
    WHERE pp.page_id=p.id
  ),'[]'::jsonb)
FROM pages p
LEFT JOIN users u ON u.id=p.updated_by
LEFT JOIN wiki_groups owner ON owner.id=p.owner_group_id
WHERE p.slug=$1 AND p.deleted_at IS NULL`

// completePageQueryRower is the query-row capability shared by the pool and import transactions.
type completePageQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// GetPage returns one complete page in a single PostgreSQL round trip.
func (s *Store) GetPage(ctx context.Context, slug string) (domain.Page, error) {
	return getPage(ctx, s.pool, slug)
}

// GetPage returns one complete page inside the import transaction.
func (s *ImportStore) GetPage(ctx context.Context, slug string) (domain.Page, error) {
	return getPage(ctx, s.tx, slug)
}

// getPage loads one complete page through the explicitly supplied query boundary.
func getPage(ctx context.Context, query completePageQueryRower, slug string) (domain.Page, error) {
	return scanCompletePage(query.QueryRow(ctx, completePageSelect, slug))
}

// scanCompletePage scans the full page projection and restores its structured metadata.
func scanCompletePage(row pgx.Row) (domain.Page, error) {
	var page domain.Page
	var pluginUsage json.RawMessage
	var renderedContents json.RawMessage
	var groups json.RawMessage
	var properties json.RawMessage

	err := row.Scan(
		&page.ID,
		&page.Slug,
		&page.Title,
		&page.Icon,
		&page.Markdown,
		&page.CreatedBy,
		&page.UpdatedBy,
		&page.Author,
		&page.CreatedAt,
		&page.UpdatedAt,
		&page.ViewCount,
		&page.Tags,
		&page.Status,
		&pluginUsage,
		&page.Language,
		&page.OwnerGroupID,
		&page.OwnerGroup,
		&page.LastReviewedAt,
		&page.ReviewIntervalDays,
		&page.DeprecatedTarget,
		&page.Render.HTML,
		&renderedContents,
		&page.Render.Fingerprint,
		&groups,
		&properties,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Page{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Page{}, err
	}

	decodePagePluginUsage(&page, pluginUsage)
	decodePageRenderContents(&page, renderedContents)
	if err := decodePageCollections(&page, groups, properties); err != nil {
		return domain.Page{}, err
	}
	return page, nil
}

// decodePageRenderContents restores optional rebuildable render metadata and discards corrupt artifacts.
func decodePageRenderContents(page *domain.Page, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	if err := json.Unmarshal(raw, &page.Render.Contents); err != nil {
		// Render artifacts are derived data. Corrupt metadata must fall back to
		// the canonical Markdown instead of making the page unavailable.
		page.Render = domain.PageRender{}
	}
}

// decodePageCollections restores persisted group and property collections attached to a page.
func decodePageCollections(page *domain.Page, groups, properties json.RawMessage) error {
	if err := json.Unmarshal(groups, &page.Groups); err != nil {
		return fmt.Errorf("decode page groups: %w", err)
	}
	if len(page.Groups) == 0 {
		page.Groups = nil
	}

	if err := json.Unmarshal(properties, &page.Properties); err != nil {
		return fmt.Errorf("decode page properties: %w", err)
	}
	if len(page.Properties) == 0 {
		page.Properties = nil
	}
	return nil
}

// decodePagePluginUsage restores optional rebuildable plugin-usage metadata.
func decodePagePluginUsage(page *domain.Page, raw json.RawMessage) {
	if len(raw) == 0 {
		return
	}
	var usage pluginusage.Index
	if json.Unmarshal(raw, &usage) == nil {
		page.PluginUsage = &usage
	}
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
		pageSummarySelect+`
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

	return collectPageSummaries(rows)
}

// NavigationPages returns the minimal page data required to build navigation.
func (s *Store) NavigationPages(ctx context.Context) ([]domain.Page, error) {
	rows, err := s.pool.Query(ctx, `
SELECT p.id,p.slug,p.title,coalesce(i.icon,'')
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
		if err := rows.Scan(&page.ID, &page.Slug, &page.Title, &page.Icon); err != nil {
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

// collectPageSummaries scans body-free page rows.
func collectPageSummaries(rows pgx.Rows) ([]domain.Page, error) {
	var out []domain.Page
	for rows.Next() {
		page, err := scanPageSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, page)
	}
	return out, rows.Err()
}
