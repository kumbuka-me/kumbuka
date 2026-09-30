package pagecontent

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// Rebuilds coordinates administrator-triggered and plugin-triggered page render rebuilds.
type Rebuilds struct {
	// catalog lists every non-deleted page that may need a rebuilt render artifact.
	catalog pageInventory
	// pages loads canonical Markdown for one page.
	pages pageContent
	// artifacts persists reusable rendered-page artifacts.
	artifacts artifactStore
	// renderer produces the current renderer/plugin representation.
	renderer *md.Renderer
	// logger records rebuild progress and failures.
	logger *slog.Logger

	// mu protects the pending rebuild state.
	mu sync.Mutex
	// dirty reports whether another full rebuild has been requested.
	dirty bool
	// running reports whether a worker is draining pending rebuilds.
	running bool
	// reason describes the latest nonempty rebuild trigger.
	reason string

	// renderMu serializes manual and full rebuilds to bound rendering memory.
	renderMu sync.Mutex
}

// NewRebuilds constructs page render rebuild coordination around existing application capabilities.
func NewRebuilds(
	catalog pageInventory,
	pages pageContent,
	artifacts artifactStore,
	renderer *md.Renderer,
	logger *slog.Logger,
) *Rebuilds {
	return &Rebuilds{
		catalog:   catalog,
		pages:     pages,
		artifacts: artifacts,
		renderer:  renderer,
		logger:    logger.With("component", "page-render-rebuild"),
	}
}

// Available reports whether all dependencies required for page render rebuilds are configured.
func (c *Rebuilds) Available() bool {
	return c != nil && c.catalog != nil && c.pages != nil && c.artifacts != nil && c.renderer != nil
}

// RebuildPage renders one page from canonical Markdown and replaces its reusable artifact.
// Administrator-triggered rebuilds are serialized so a manual rebuild cannot overlap
// a potentially memory-heavy all-pages rebuild.
func (c *Rebuilds) RebuildPage(ctx context.Context, slug string) error {
	if !c.Available() {
		return errors.New("page render rebuild is unavailable")
	}

	c.renderMu.Lock()
	defer c.renderMu.Unlock()

	return c.rebuildPage(ctx, slug)
}

// rebuildPage performs one rebuild while the caller owns renderMu.
func (c *Rebuilds) rebuildPage(ctx context.Context, slug string) error {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return domain.ErrNotFound
	}

	page, err := c.pages.GetPage(ctx, slug)
	if err != nil {
		return err
	}

	usage := c.renderer.AnalyzeUsage(page.Markdown)
	render := domain.PageRender{}
	options := md.DefaultOptions()
	if c.renderer.CanPersist(page.Markdown, &usage) {
		rendered, renderErr := c.renderer.RenderPageResolvedWithFunctions(
			page.Markdown,
			md.Slug,
			options,
			md.Functions{
				Context:     ctx,
				Locale:      "en",
				PluginUsage: &usage,
			},
		)
		if renderErr != nil {
			return renderErr
		}
		if artifact, ok := Artifact(rendered, c.renderer.RenderFingerprint(options)); ok {
			render = artifact
		}
	}

	return c.artifacts.SavePageRender(ctx, page.ID, page.UpdatedAt, render)
}

// RebuildAll rebuilds every current page and continues past page-local failures.
func (c *Rebuilds) RebuildAll(ctx context.Context) (completed, failed int) {
	if !c.Available() {
		return 0, 1
	}

	pages, err := c.catalog.PageInventory(ctx)
	if err != nil {
		c.logger.Error("list pages for render rebuild", "event", "page_render_rebuild_list_failed", "error", err)
		return 0, 1
	}

	// Hold the rebuild lock for the complete batch. This guarantees that pages are
	// rendered one at a time and prevents manual rebuilds from increasing render
	// concurrency while a Render all job is running.
	c.renderMu.Lock()
	defer c.renderMu.Unlock()

	for _, page := range pages {
		if err := ctx.Err(); err != nil {
			c.logger.Warn("page render rebuild canceled", "event", "page_render_rebuild_canceled", "error", err)
			return completed, failed + 1
		}
		if err := c.rebuildPage(ctx, page.Slug); err != nil {
			failed++
			c.logger.Error(
				"rebuild page render",
				"event", "page_render_rebuild_page_failed",
				"slug", page.Slug,
				"error", err,
			)
			continue
		}
		completed++
	}

	return completed, failed
}

// MarkAllDirty records that cached page renders need a rebuild without starting work yet.
func (c *Rebuilds) MarkAllDirty(reason string) {
	if !c.Available() {
		return
	}

	c.mu.Lock()
	c.dirty = true
	if reason = strings.TrimSpace(reason); reason != "" {
		c.reason = reason
	}
	c.mu.Unlock()
}

// QueueAll marks all cached page renders dirty and starts the background worker.
func (c *Rebuilds) QueueAll(reason string) {
	c.MarkAllDirty(reason)
	c.FlushPending()
}

// FlushPending starts one background rebuild worker when deferred changes are pending.
func (c *Rebuilds) FlushPending() {
	if !c.Available() {
		return
	}

	c.mu.Lock()
	if !c.dirty || c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	go c.run()
}

// run drains pending rebuild requests and coalesces changes arriving while a rebuild is in progress.
func (c *Rebuilds) run() {
	for {
		c.mu.Lock()
		if !c.dirty {
			c.running = false
			c.mu.Unlock()
			return
		}
		reason := c.reason
		c.dirty = false
		c.reason = ""
		c.mu.Unlock()

		c.logger.Info("page render rebuild started", "event", "page_render_rebuild_started", "reason", reason)
		completed, failed := c.RebuildAll(context.Background())
		c.logger.Info(
			"page render rebuild finished",
			"event", "page_render_rebuild_finished",
			"reason", reason,
			"completed", completed,
			"failed", failed,
		)
	}
}

// pageInventory lists the pages eligible for render-cache rebuilding.
type pageInventory interface {
	PageInventory(context.Context) ([]domain.Page, error)
}

// pageContent loads the canonical Markdown and version of one page.
type pageContent interface {
	GetPage(context.Context, string) (domain.Page, error)
}

// artifactStore persists artifacts only while their source page version is current.
type artifactStore interface {
	SavePageRender(context.Context, int64, time.Time, domain.PageRender) error
}
