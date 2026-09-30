package endpoint

import (
	"context"
	"github.com/kumbuka-me/kumbuka/internal/pagecontent"
	"log/slog"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// renderPageContent reuses a matching render artifact or renders and persists a fresh artifact.
func renderPageContent(
	ctx context.Context,
	locale string,
	page domain.Page,
	options md.Options,
	capabilities map[string]plugin.Capability,
	renderer *md.Renderer,
	artifacts pageRenderArtifactStore,
	logger *slog.Logger,
) (md.RenderedPage, error) {
	fingerprint := renderer.RenderFingerprint(options)
	persistable := (locale == "" || locale == "en") && renderer.CanPersist(page.Markdown, page.PluginUsage)
	if persistable && page.Render.Fingerprint == fingerprint {
		stop := measurePageStage(ctx, "render_artifact_hit")
		rendered := pagecontent.FromArtifact(page.Render)
		stop()
		return rendered, nil
	}

	stop := measurePageStage(ctx, "markdown")
	rendered, err := renderer.RenderPageResolvedWithFunctions(
		page.Markdown,
		md.Slug,
		options,
		md.Functions{
			Context:         ctx,
			Locale:          locale,
			PluginUsage:     page.PluginUsage,
			Capabilities:    capabilities,
			DeferMacro:      renderer.ShouldDeferMacro,
			DeferredVersion: strconv.FormatInt(page.UpdatedAt.UnixNano(), 10),
		},
	)
	stop()
	if err != nil {
		return md.RenderedPage{}, err
	}

	if !persistable {
		return rendered, nil
	}
	artifact, ok := pagecontent.Artifact(rendered, fingerprint)
	if !ok {
		return rendered, nil
	}

	stop = measurePageStage(ctx, "render_artifact_store")
	err = artifacts.SavePageRender(ctx, page.ID, page.UpdatedAt, artifact)
	stop()
	if err != nil {
		logger.Warn(
			"store page render artifact",
			"event", "page_render_store_failed",
			"slug", page.Slug,
			"error", err,
		)
	}

	return rendered, nil
}

// markBrokenWikiLinks adds the broken-link class to rendered wiki links whose targets do not exist.
func markBrokenWikiLinks(renderedHTML string, links []domain.PageLink) string {
	for _, link := range links {
		if link.Exists {
			continue
		}
		renderedHTML = strings.ReplaceAll(
			renderedHTML,
			`<a href="/pages/`+link.TargetSlug+`"`,
			`<a class="wiki-link-broken" href="/pages/`+link.TargetSlug+`"`,
		)
	}

	return renderedHTML
}
