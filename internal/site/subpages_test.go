package site

import (
	"testing"

	md "github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/navigation"
	"github.com/kumbuka-me/kumbuka/internal/plugincap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticSubpagesUseStaticPageURLs(t *testing.T) {
	t.Parallel()

	tree := navigation.Build([]navigation.Page{
		{Slug: "guide", Title: "Guide"},
		{Slug: "guide/install", Title: "Install"},
	}, navigation.Options{})
	nodes := plugincap.Navigation(navigation.Children(tree, "guide"), func(slug string) string {
		return pageURL("/docs/", slug)
	})

	renderer, _ := testPluginMarkdownRenderer(t, "subpages")
	rendered, err := renderer.RenderPageResolvedWithFunctions(`{{subpages title="Related pages"}}`, md.Slug, md.DefaultOptions(), md.Functions{Capabilities: plugincap.Capabilities(nil, nodes)})
	html := rendered.HTML

	require.NoError(t, err)
	assert.Contains(t, html, "Related pages")
	assert.Contains(t, html, `href="/docs/guide/install/"`)
	assert.NotContains(t, html, `href="/docs/guide/"`)
}
