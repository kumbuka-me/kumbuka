package markdown

import "github.com/kumbuka-me/kumbuka/pkg/plugin"

// Options controls core Markdown rendering behavior.
type Options struct {
	pipeline *renderPipeline
	depth    int
	// annotations contains request-local opaque plugin substitutions for an annotated render pass.
	annotations []plugin.Replacement
	// WikiLinks enables [[Wiki Link]] resolution.
	WikiLinks bool
	// WikiLinkPrefix is prepended to resolved wiki-link targets. Empty uses /pages/.
	WikiLinkPrefix string
}

// DefaultOptions returns the default core Markdown rendering behavior.
func DefaultOptions() Options {
	return Options{
		WikiLinks:      true,
		WikiLinkPrefix: "/pages/",
	}
}
