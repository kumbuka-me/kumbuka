package pagecontent

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type adminRenderCatalogStub struct {
	pages []domain.Page
	err   error
}

func (s adminRenderCatalogStub) PageInventory(context.Context) ([]domain.Page, error) {
	return s.pages, s.err
}

type adminRenderPageStoreStub struct {
	pages map[string]domain.Page
	err   error
}

func (s adminRenderPageStoreStub) GetPage(_ context.Context, slug string) (domain.Page, error) {
	if s.err != nil {
		return domain.Page{}, s.err
	}
	page, ok := s.pages[slug]
	if !ok {
		return domain.Page{}, domain.ErrNotFound
	}
	return page, nil
}

func (s adminRenderPageStoreStub) ResolvePageLinks(context.Context, []string) ([]domain.PageLink, error) {
	return nil, s.err
}

type adminRenderArtifactStoreStub struct {
	pageID    int64
	updatedAt time.Time
	render    domain.PageRender
	calls     int
	err       error
}

func (s *adminRenderArtifactStoreStub) SavePageRender(_ context.Context, pageID int64, updatedAt time.Time, render domain.PageRender) error {
	s.calls++
	s.pageID = pageID
	s.updatedAt = updatedAt
	s.render = render
	return s.err
}

func TestAdminRenderRebuildPageStoresStableArtifact(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	page := domain.Page{ID: 7, Slug: "guide", Markdown: "# Guide\n\nStatic content.", UpdatedAt: updatedAt}
	artifacts := &adminRenderArtifactStoreStub{}
	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")
	rebuilds := NewRebuilds(
		adminRenderCatalogStub{},
		adminRenderPageStoreStub{pages: map[string]domain.Page{"guide": page}},
		artifacts,
		renderer,
		slog.Default(),
		"",
	)

	err := rebuilds.RebuildPage(context.Background(), "guide")

	require.NoError(t, err)
	assert.Equal(t, 1, artifacts.calls)
	assert.Equal(t, page.ID, artifacts.pageID)
	assert.Equal(t, updatedAt, artifacts.updatedAt)
	assert.Contains(t, artifacts.render.HTML, `<h1 id="guide">Guide</h1>`)
	assert.NotEmpty(t, artifacts.render.Fingerprint)
	require.Len(t, artifacts.render.Contents, 1)
	assert.Equal(t, "guide", artifacts.render.Contents[0].ID)
}

func TestAdminRenderRebuildPageClearsUnpersistableArtifact(t *testing.T) {
	t.Parallel()

	page := domain.Page{ID: 8, Slug: "dynamic", Markdown: "{{var:environment}}", UpdatedAt: time.Now()}
	artifacts := &adminRenderArtifactStoreStub{}
	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")
	rebuilds := NewRebuilds(
		adminRenderCatalogStub{},
		adminRenderPageStoreStub{pages: map[string]domain.Page{"dynamic": page}},
		artifacts,
		renderer,
		slog.Default(),
		"",
	)

	err := rebuilds.RebuildPage(context.Background(), "dynamic")

	require.NoError(t, err)
	assert.Equal(t, 1, artifacts.calls)
	assert.Empty(t, artifacts.render.HTML)
	assert.Empty(t, artifacts.render.Fingerprint)
	assert.Empty(t, artifacts.render.Contents)
}

func TestAdminRenderRebuildAllContinuesPastPageFailure(t *testing.T) {
	t.Parallel()

	pages := []domain.Page{{Slug: "one"}, {Slug: "missing"}, {Slug: "two"}}
	store := adminRenderPageStoreStub{pages: map[string]domain.Page{
		"one": {ID: 1, Slug: "one", Markdown: "# One", UpdatedAt: time.Now()},
		"two": {ID: 2, Slug: "two", Markdown: "# Two", UpdatedAt: time.Now()},
	}}
	artifacts := &adminRenderArtifactStoreStub{}
	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")
	rebuilds := NewRebuilds(adminRenderCatalogStub{pages: pages}, store, artifacts, renderer, slog.Default(), "")

	completed, failed := rebuilds.RebuildAll(context.Background())

	assert.Equal(t, 2, completed)
	assert.Equal(t, 1, failed)
	assert.Equal(t, 2, artifacts.calls)
}

func TestAdminRenderRebuildUnavailable(t *testing.T) {
	t.Parallel()

	rebuilds := NewRebuilds(nil, nil, nil, nil, slog.Default(), "")
	err := rebuilds.RebuildPage(context.Background(), "guide")

	require.ErrorContains(t, err, "page render rebuild is unavailable")
	assert.False(t, rebuilds.Available())
}

type blockingAdminRenderArtifactStoreStub struct {
	entered chan struct{}
	release chan struct{}
}

type notifyingAdminRenderArtifactStoreStub struct {
	// saved signals each persisted render artifact.
	saved chan struct{}
}

func (s *notifyingAdminRenderArtifactStoreStub) SavePageRender(context.Context, int64, time.Time, domain.PageRender) error {
	s.saved <- struct{}{}
	return nil
}

func TestQueuedRenderRebuildUsesOwnedWorkerContext(t *testing.T) {
	page := domain.Page{ID: 1, Slug: "guide", Markdown: "# Guide", UpdatedAt: time.Now()}
	artifacts := &notifyingAdminRenderArtifactStoreStub{saved: make(chan struct{}, 1)}
	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")
	rebuilds := NewRebuilds(
		adminRenderCatalogStub{pages: []domain.Page{page}},
		adminRenderPageStoreStub{pages: map[string]domain.Page{page.Slug: page}},
		artifacts,
		renderer,
		slog.Default(),
		"",
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rebuilds.Run(ctx)
		close(done)
	}()

	rebuilds.QueueAll("test")
	select {
	case <-artifacts.saved:
	case <-time.After(time.Second):
		t.Fatal("queued rebuild did not run")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("rebuild worker did not stop after cancellation")
	}
}

func (s *blockingAdminRenderArtifactStoreStub) SavePageRender(context.Context, int64, time.Time, domain.PageRender) error {
	s.entered <- struct{}{}
	<-s.release
	return nil
}

func TestAdminRenderRebuildAllHoldsExclusiveRenderLock(t *testing.T) {
	page := domain.Page{ID: 1, Slug: "one", Markdown: "# One", UpdatedAt: time.Now()}
	artifacts := &blockingAdminRenderArtifactStoreStub{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	renderer := md.NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("test", "abc")
	rebuilds := NewRebuilds(
		adminRenderCatalogStub{pages: []domain.Page{{Slug: page.Slug}}},
		adminRenderPageStoreStub{pages: map[string]domain.Page{page.Slug: page}},
		artifacts,
		renderer,
		slog.Default(),
		"",
	)

	type result struct {
		completed int
		failed    int
	}
	done := make(chan result, 1)
	go func() {
		completed, failed := rebuilds.RebuildAll(context.Background())
		done <- result{completed: completed, failed: failed}
	}()

	select {
	case <-artifacts.entered:
	case <-time.After(time.Second):
		t.Fatal("render-all did not reach artifact persistence")
	}

	if rebuilds.renderMu.TryLock() {
		rebuilds.renderMu.Unlock()
		t.Fatal("render-all must hold the exclusive render lock for the complete batch")
	}

	close(artifacts.release)
	select {
	case result := <-done:
		assert.Equal(t, 1, result.completed)
		assert.Zero(t, result.failed)
	case <-time.After(time.Second):
		t.Fatal("render-all did not finish after releasing artifact persistence")
	}
}
