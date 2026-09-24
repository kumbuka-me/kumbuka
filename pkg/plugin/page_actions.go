package plugin

import (
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// PageActionContribution is one safe local page action rendered by the host.
type PageActionContribution struct {
	// PluginID identifies the plugin that owns the action.
	PluginID string
	// ModuleID identifies the page-action module within the plugin.
	ModuleID string
	// Name is the human-readable action label.
	Name string
	// Kind selects link or host-dialog presentation.
	Kind PageActionKind
	// Description is optional help text shown by the host.
	Description string
	// Icon is the optional host icon rendered for the action.
	Icon string
	// URL is the resolved local application path for the current page.
	URL string
	// Order controls deterministic placement among plugin page actions.
	Order int
}

// PageActions returns active declarative page actions resolved for one current page.
func (m *Manager) PageActions(pageID int64, slug string) []PageActionContribution {
	return pageActions(m.Plugins(), pageID, slug)
}

// pageActions resolves page-action modules from stable loaded-plugin metadata.
func pageActions(plugins []LoadedPlugin, pageID int64, slug string) []PageActionContribution {
	var result []PageActionContribution
	for _, item := range plugins {
		if !item.Enabled {
			continue
		}
		for _, module := range item.Manifest.Modules {
			if ModuleType(module.Type) != ModuleTypePageAction {
				continue
			}
			result = append(result, PageActionContribution{
				PluginID:    item.Manifest.ID,
				ModuleID:    module.ID,
				Name:        module.Name,
				Kind:        pageActionKind(module.Kind),
				Description: module.Description,
				Icon:        module.Icon,
				URL:         resolvePageActionURL(module.URL, pageID, slug),
				Order:       module.Order,
			})
		}
	}

	sort.SliceStable(result, func(i, j int) bool { return result[i].Order < result[j].Order })
	return result
}

// resolvePageActionURL expands validated page placeholders using path-safe values.
func resolvePageActionURL(template string, pageID int64, slug string) string {
	parts := strings.Split(slug, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	resolved := strings.ReplaceAll(template, "${slug}", strings.Join(parts, "/"))
	return strings.ReplaceAll(resolved, "${id}", strconv.FormatInt(pageID, 10))
}

// pageActionKind applies the declarative default for page actions.
func pageActionKind(kind string) PageActionKind {
	if kind == "" {
		return PageActionLink
	}
	return PageActionKind(kind)
}
