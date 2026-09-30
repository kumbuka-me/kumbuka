package pagecontent

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

// Preparer derives persisted plugin usage from canonical Markdown.
type Preparer struct {
	// renderer analyzes plugin usage without running render modules on the save path.
	renderer *markdown.Renderer
}

// New constructs page-content preparation around the active Markdown renderer.
func New(renderer *markdown.Renderer) *Preparer {
	return &Preparer{renderer: renderer}
}

// Prepare derives plugin usage and invalidates the previous artifact. Rendering is deliberately excluded from the save request: the first page read lazily stores a safe artifact, and administrator rebuilds can precompute them.
func (p *Preparer) Prepare(_ context.Context, source string) (*pluginusage.Index, domain.PageRender, error) {
	if p == nil || p.renderer == nil {
		return nil, domain.PageRender{}, nil
	}

	usage := p.renderer.AnalyzeUsage(source)
	return &usage, domain.PageRender{}, nil
}
