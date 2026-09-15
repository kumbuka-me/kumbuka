package site

import (
	"context"
	"fmt"

	md "github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/pluginproject"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// projectRenderer resolves declared project packages without starting their
// runtimes, selects only packages that can affect the discovered Markdown, then
// creates a renderer from that minimal package set.
func projectRenderer(ctx context.Context, filename string, pages []sourcePage) (*md.Renderer, error) {
	file, err := pluginproject.Load(filename)
	if err != nil {
		return nil, err
	}
	if len(file.Plugins) == 0 {
		return md.NewWithPluginPackages(ctx, nil, nil)
	}

	resolver, err := pluginproject.NewResolver(plugins.Packages)
	if err != nil {
		return nil, err
	}
	resolved, err := resolver.Resolve(ctx, file.Plugins)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", filename, err)
	}

	manifests := make([]pluginpackage.Manifest, 0, len(resolved))
	for _, item := range resolved {
		manifests = append(manifests, item.Manifest)
	}
	sources := make([]string, 0, len(pages))
	for _, page := range pages {
		sources = append(sources, page.Markdown)
	}
	required := md.RequiredPluginIDs(sources, manifests)
	selected, err := pluginproject.Select(resolved, required)
	if err != nil {
		return nil, err
	}

	archives := make([][]byte, 0, len(selected))
	ids := make([]string, 0, len(selected))
	for _, item := range selected {
		archives = append(archives, item.Archive)
		ids = append(ids, item.Manifest.ID)
	}
	return md.NewWithPluginPackages(ctx, archives, ids)
}
