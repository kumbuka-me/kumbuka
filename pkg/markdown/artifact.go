package markdown

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/sdk"
)

const renderArtifactVersion = 1

// SetArtifactBuild identifies the core renderer build used by persisted page artifacts. Release builds should pass their version/commit so an upgrade invalidates old HTML.
func (r *Renderer) SetArtifactBuild(version, commit string) {
	version = strings.TrimSpace(version)
	commit = strings.TrimSpace(commit)
	if version == "dev" && (commit == "" || commit == "none") {
		// Development binaries do not carry a stable source identity. Keep artifacts
		// reusable within one process, but force a safe lazy rebuild after restart.
		r.artifactBuild = fmt.Sprintf("dev@%d", time.Now().UnixNano())
		return
	}
	r.artifactBuild = version + "@" + commit
}

// RenderFingerprint identifies all stable renderer inputs that can change persisted HTML.
func (r *Renderer) RenderFingerprint(options Options) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash,
		"artifact=%d\napi=%d\nbuild=%s\nwiki_links=%t\nwiki_prefix=%s\n",
		renderArtifactVersion,
		sdk.Version,
		r.artifactBuild,
		options.WikiLinks,
		options.WikiLinkPrefix,
	)
	if r.manager == nil {
		return hex.EncodeToString(hash.Sum(nil))
	}
	plugins := r.manager.Plugins()
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Manifest.ID < plugins[j].Manifest.ID })
	for _, item := range plugins {
		if !item.Enabled {
			continue
		}
		_, _ = fmt.Fprintf(hash, "plugin=%s\nversion=%s\ndigest=%x\n", item.Manifest.ID, item.Manifest.Version, item.Digest)
		keys := make([]string, 0, len(item.Settings))
		for key := range item.Settings {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			_, _ = fmt.Fprintf(hash, "setting=%s:%t\n", key, item.Settings[key])
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// CanPersist reports whether a page can be rendered once without capturing request-local authorization or mutable plugin resource data.
func (r *Renderer) CanPersist(source string, usage *pluginusage.Index) bool {
	if sourceHasDynamicMarkdown(source) {
		return false
	}
	if r.manager == nil {
		return true
	}
	plan, release := r.registry.AcquireRenderPlan()
	defer release()
	usage = currentOrAnalyzedUsage(source, usage, plan)
	selected := selectedUsageModules(usage)
	return !r.hasSelectedDynamicPluginModule(selected)
}

// sourceHasDynamicMarkdown reports whether source contains request-time Markdown constructs outside code fences.
func sourceHasDynamicMarkdown(source string) bool {
	scanner := newUsageScanner(source)
	for _, line := range scanner.outside {
		if strings.Contains(line, "{{") {
			return true
		}
	}
	return false
}

// currentOrAnalyzedUsage returns a source-current usage index.
func currentOrAnalyzedUsage(source string, usage *pluginusage.Index, plan *plugin.RenderPlan) *pluginusage.Index {
	if currentUsageIndex(usage, plan, source) {
		return usage
	}
	index := analyzeUsage(source, plan)
	return &index
}

// selectedUsageModules indexes source-selected plugin modules by plugin and module ID.
func selectedUsageModules(usage *pluginusage.Index) map[string]bool {
	selected := make(map[string]bool, len(usage.Modules))
	for _, module := range usage.Modules {
		selected[module.PluginID+"\x00"+module.ModuleID] = true
	}
	return selected
}

// hasSelectedDynamicPluginModule reports whether a dynamic-read plugin can affect this page.
func (r *Renderer) hasSelectedDynamicPluginModule(selected map[string]bool) bool {
	for _, item := range r.manager.Plugins() {
		if !item.Enabled || !hasDynamicReadPermission(item.Manifest.Permissions) {
			continue
		}
		if manifestHasSelectedDynamicModule(item, selected) {
			return true
		}
	}
	return false
}

// manifestHasSelectedDynamicModule reports whether one plugin has an executable dynamic module relevant to the page.
func manifestHasSelectedDynamicModule(item plugin.LoadedPlugin, selected map[string]bool) bool {
	for _, module := range item.Manifest.Modules {
		if !renderExecutableModule(module.Type) {
			continue
		}
		if len(module.Usage) == 0 || selected[item.Manifest.ID+"\x00"+module.ID] {
			return true
		}
	}
	return false
}

// hasDynamicReadPermission reports whether a plugin manifest grants a dynamic read capability.
func hasDynamicReadPermission(permissions []string) bool {
	for _, permission := range permissions {
		if dynamicReadPermission(permission) {
			return true
		}
	}
	return false
}

// dynamicReadPermission reports whether a permission can make rendered output depend on mutable request-time data.
func dynamicReadPermission(permission string) bool {
	switch permission {
	case "pages:read", "pages:content", "attachments:read", "settings:read", "storage:read", "network:http":
		return true
	default:
		return false
	}
}

// renderExecutableModule renders one executable plugin module with the supplied context.
func renderExecutableModule(moduleType string) bool {
	switch moduleType {
	case "renderer-extension", "code-highlighter", "content-substitution", "macro":
		return true
	default:
		return false
	}
}
