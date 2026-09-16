package plugin

import (
	"fmt"
	"io/fs"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

var _ icons.ResourceProvider = (*Manager)(nil)

// BrowserContribution is public asset metadata; it contains no settings or
// capabilities. The digest pins every module and auxiliary asset to one version.
type BrowserContribution struct {
	// PluginID identifies the plugin that owns the contribution.
	PluginID string `json:"plugin_id"`
	// ModuleID identifies the module within its plugin.
	ModuleID string `json:"module_id"`
	// Name is the human-readable name.
	Name string `json:"name"`
	// Version identifies the associated plugin version.
	Version string `json:"version"`
	// Digest stores the content digest used for identity and caching.
	Digest string `json:"digest"`
	// JavaScript is the validated path of the module JavaScript asset.
	JavaScript string `json:"javascript"`
	// CSS is the validated path of the optional module stylesheet.
	CSS string `json:"css,omitempty"`
}

// ContentStyleContribution is safe parent-document stylesheet metadata for one
// active plugin version. Core still filters the stylesheet before publishing it.
type ContentStyleContribution struct {
	// PluginID identifies the plugin that owns the contribution.
	PluginID string
	// ModuleID identifies the module within its plugin.
	ModuleID string
	// Digest stores the content digest used for identity and caching.
	Digest string
	// CSS is the validated package-relative stylesheet asset path.
	CSS string
}

// CodeHighlighterContribution is scoped stylesheet metadata for an active highlighter.
type CodeHighlighterContribution struct {
	// PluginID identifies the plugin that owns the contribution.
	PluginID string
	// ModuleID identifies the module within its plugin.
	ModuleID string
	// Digest stores the content digest used for identity and caching.
	Digest string
	// CSS is the optional package-relative stylesheet asset path.
	CSS string
}

// IconResourceVersion returns a stable generation key for enabled icon providers.
// Unrelated plugin lifecycle changes therefore do not invalidate the icon catalog.
func (m *Manager) IconResourceVersion() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var version strings.Builder
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled || !hasModuleType(item.metadata.Manifest, "icon-resource") {
			continue
		}
		version.WriteString(id)
		version.WriteByte('=')
		_, _ = fmt.Fprintf(&version, "%x", item.metadata.Digest)
		version.WriteByte('\n')
	}
	return version.String()
}

// IconResources returns assets explicitly declared by enabled icon-resource modules.
// Package validation guarantees that every declared asset exists and is bounded.
func (m *Manager) IconResources() ([]icons.Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	resources := make([]icons.Resource, 0)
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled || !hasModuleType(item.metadata.Manifest, "icon-resource") {
			continue
		}

		var pkg *pluginpackage.Package
		for _, module := range item.metadata.Manifest.Modules {
			if module.Type != "icon-resource" {
				continue
			}
			if pkg == nil {
				loaded, err := pluginpackage.Read(item.archive)
				if err != nil {
					return nil, err
				}
				pkg = loaded
			}
			data, err := pkg.Asset(module.Asset)
			if err != nil {
				return nil, err
			}
			resources = append(resources, icons.Resource{
				Source: module.Name,
				Data:   data,
			})
		}
	}
	return resources, nil
}

// hasModuleType reports whether a manifest declares at least one module of the requested type.
func hasModuleType(manifest pluginpackage.Manifest, moduleType string) bool {
	for _, module := range manifest.Modules {
		if module.Type == moduleType {
			return true
		}
	}
	return false
}

// BrowserModules returns browser modules contributed by active plugins.
func (m *Manager) BrowserModules() []BrowserContribution {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]BrowserContribution, 0)
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled {
			continue
		}
		for _, module := range item.metadata.Manifest.Modules {
			if module.Type != "browser-module" {
				continue
			}
			result = append(result, BrowserContribution{PluginID: id, ModuleID: module.ID, Name: item.metadata.Manifest.Name, Version: item.metadata.Manifest.Version, Digest: fmt.Sprintf("%x", item.metadata.Digest), JavaScript: module.JavaScript, CSS: module.CSS})
		}
	}
	return result
}

// CodeHighlighters returns stylesheet metadata for the active highlighter provider.
func (m *Manager) CodeHighlighters() []CodeHighlighterContribution {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]CodeHighlighterContribution, 0, 1)
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled {
			continue
		}
		for _, module := range item.metadata.Manifest.Modules {
			if module.Type != "code-highlighter" {
				continue
			}
			result = append(result, CodeHighlighterContribution{
				PluginID: id,
				ModuleID: module.ID,
				Digest:   fmt.Sprintf("%x", item.metadata.Digest),
				CSS:      module.CSS,
			})
		}
	}

	return result
}

// ContentStyles returns parent-document style contributions from active plugins.
func (m *Manager) ContentStyles() []ContentStyleContribution {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]ContentStyleContribution, 0)
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled {
			continue
		}
		for _, module := range item.metadata.Manifest.Modules {
			if module.Type != "content-style" {
				continue
			}
			result = append(result, ContentStyleContribution{
				PluginID: id,
				ModuleID: module.ID,
				Digest:   fmt.Sprintf("%x", item.metadata.Digest),
				CSS:      module.CSS,
			})
		}
	}

	return result
}

// BrowserAsset serves bytes from an enabled, exact-version package only. There
// is no filesystem extraction, and lifecycle changes invalidate old URLs.
func (m *Manager) BrowserAsset(id, digest, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.loaded[id]
	if !ok || !item.metadata.Enabled || fmt.Sprintf("%x", item.metadata.Digest) != digest {
		return nil, fs.ErrNotExist
	}
	hasBrowser := false
	for _, module := range item.metadata.Manifest.Modules {
		if module.Type == "browser-module" {
			hasBrowser = true
			break
		}
	}
	if !hasBrowser {
		return nil, fs.ErrNotExist
	}
	pkg, err := pluginpackage.Read(item.archive)
	if err != nil {
		return nil, err
	}
	return pkg.Asset(name)
}

// CodeHighlighterAsset returns an asset declared by an active code-highlighter module.
func (m *Manager) CodeHighlighterAsset(id, digest, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.declaredAsset(id, digest, name, "code-highlighter")
}

// ContentStyleAsset returns an asset declared by an active content-style module.
func (m *Manager) ContentStyleAsset(id, digest, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.declaredAsset(id, digest, name, "content-style")
}

// declaredAsset returns one exact-version asset declared by moduleType.
func (m *Manager) declaredAsset(id, digest, name, moduleType string) ([]byte, error) {
	item, ok := m.loaded[id]
	if !ok || !item.metadata.Enabled || fmt.Sprintf("%x", item.metadata.Digest) != digest {
		return nil, fs.ErrNotExist
	}
	declared := false
	for _, module := range item.metadata.Manifest.Modules {
		if module.Type == moduleType && (module.JavaScript == name || module.CSS == name) {
			declared = true
			break
		}
	}
	if !declared {
		return nil, fs.ErrNotExist
	}
	pkg, err := pluginpackage.Read(item.archive)
	if err != nil {
		return nil, err
	}
	return pkg.Asset(name)
}

// BrowserAssetNames returns asset paths declared by active browser modules.
func (m *Manager) BrowserAssetNames(id, digest string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.loaded[id]
	if !ok || !item.metadata.Enabled || fmt.Sprintf("%x", item.metadata.Digest) != digest {
		return nil, fs.ErrNotExist
	}
	pkg, err := pluginpackage.Read(item.archive)
	if err != nil {
		return nil, err
	}
	return pkg.AssetNames(), nil
}
