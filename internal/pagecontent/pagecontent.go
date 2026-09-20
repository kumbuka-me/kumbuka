package pagecontent

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

// Preparer derives persisted plugin usage and reusable render artifacts from canonical Markdown.
type Preparer struct {
	renderer *md.Renderer
}

// New constructs page-content preparation around the active Markdown renderer.
func New(renderer *md.Renderer) *Preparer {
	return &Preparer{renderer: renderer}
}

// Prepare derives plugin usage and materializes stable HTML when the render is safe to persist.
func (p *Preparer) Prepare(ctx context.Context, source string) (*pluginusage.Index, domain.PageRender, error) {
	if p == nil || p.renderer == nil {
		return nil, domain.PageRender{}, nil
	}

	usage := p.renderer.AnalyzeUsage(source)
	if !p.renderer.CanPersist(source, &usage) {
		return &usage, domain.PageRender{}, nil
	}

	options := md.DefaultOptions()
	rendered, err := p.renderer.RenderPageResolvedWithFunctions(source, md.Slug, options, md.Functions{
		Context:     ctx,
		PluginUsage: &usage,
	})
	if err != nil {
		return nil, domain.PageRender{}, err
	}
	// Inspector/export metadata originates from mutable substitutions. Keep such
	// pages on the request-time path as an additional persistence guard.
	if len(rendered.Inspectors) != 0 || len(rendered.ExportFields) != 0 {
		return &usage, domain.PageRender{}, nil
	}

	contents := make([]domain.PageHeading, len(rendered.Contents))
	for index, heading := range rendered.Contents {
		contents[index] = domain.PageHeading{Level: heading.Level, ID: heading.ID, Title: heading.Title}
	}

	return &usage, domain.PageRender{
		HTML:        rendered.HTML,
		Contents:    contents,
		Fingerprint: p.renderer.RenderFingerprint(options),
	}, nil
}
