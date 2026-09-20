package endpoint

import (
	"context"
	"log/slog"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// renderPageContent reuses a matching render artifact or renders and persists a fresh artifact.
func renderPageContent(
	ctx context.Context,
	page domain.Page,
	options md.Options,
	capabilities map[string]plugin.Capability,
	renderer *md.Renderer,
	catalog pageViewCatalogService,
	logger *slog.Logger,
) (md.RenderedPage, error) {
	fingerprint := renderer.RenderFingerprint(options)
	persistable := renderer.CanPersist(page.Markdown, page.PluginUsage)
	if persistable && page.Render.Fingerprint == fingerprint {
		stop := measurePageStage(ctx, "render_artifact_hit")
		rendered := renderedPageFromArtifact(page.Render)
		stop()
		return rendered, nil
	}

	stop := measurePageStage(ctx, "markdown")
	rendered, err := renderer.RenderPageResolvedWithFunctions(
		page.Markdown,
		md.Slug,
		options,
		md.Functions{
			Context:      ctx,
			PluginUsage:  page.PluginUsage,
			Capabilities: capabilities,
		},
	)
	stop()
	if err != nil {
		return md.RenderedPage{}, err
	}

	if !persistable {
		return rendered, nil
	}
	artifact, ok := pageRenderArtifact(rendered, fingerprint)
	if !ok {
		return rendered, nil
	}

	stop = measurePageStage(ctx, "render_artifact_store")
	err = catalog.SavePageRender(ctx, page.ID, page.UpdatedAt, artifact)
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

// renderedPageFromArtifact converts a persisted render artifact into renderer output.
func renderedPageFromArtifact(render domain.PageRender) md.RenderedPage {
	contents := make([]md.Heading, len(render.Contents))
	for index, heading := range render.Contents {
		contents[index] = md.Heading{Level: heading.Level, ID: heading.ID, Title: heading.Title}
	}
	return md.RenderedPage{HTML: render.HTML, Contents: contents}
}

// pageRenderArtifact converts renderer output into a persistable artifact when it has no request-local contributions.
func pageRenderArtifact(rendered md.RenderedPage, fingerprint string) (domain.PageRender, bool) {
	if len(rendered.Inspectors) != 0 || len(rendered.ExportFields) != 0 {
		return domain.PageRender{}, false
	}
	contents := make([]domain.PageHeading, len(rendered.Contents))
	for index, heading := range rendered.Contents {
		contents[index] = domain.PageHeading{Level: heading.Level, ID: heading.ID, Title: heading.Title}
	}
	return domain.PageRender{HTML: rendered.HTML, Contents: contents, Fingerprint: fingerprint}, true
}
