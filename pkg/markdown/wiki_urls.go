package markdown

import (
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pageurl"
)

// WikiLinkResolver maps human-readable current or historical targets to stable page URLs.
func WikiLinkResolver(links []domain.PageLink) func(string) string {
	resolved := make(map[string]string, len(links))
	for _, link := range links {
		if link.Exists {
			resolved[link.TargetSlug] = pageurl.Page(link.TargetID, link.ResolvedSlug)
		}
	}
	return func(target string) string {
		target = Slug(target)
		if destination := resolved[target]; destination != "" {
			return destination
		}
		return DefaultWikiLink(target)
	}
}
