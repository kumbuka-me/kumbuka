package markdown

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/sdk"
)

const renderArtifactVersion = 1

// SetArtifactBuild identifies the core renderer build used by persisted page artifacts.
// Release builds should pass their version/commit so an upgrade invalidates old HTML.
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

// CanPersist reports whether a page can be rendered once without capturing
// request-local authorization or mutable plugin resource data.
func (r *Renderer) CanPersist(source string, usage *pluginusage.Index) bool {
	scanner := newUsageScanner(source)
	for _, line := range scanner.outside {
		// Macros, variables, snippets and includes are intentionally dynamic.
		// Keeping them out of persisted HTML also prevents saving output rendered
		// with the editor's page permissions and serving it to another reader.
		if strings.Contains(line, "{{") {
			return false
		}
	}
	if r.manager == nil {
		return true
	}
	plan, release := r.registry.AcquireRenderPlan()
	defer release()
	if !currentUsageIndex(usage, plan, source) {
		index := analyzeUsage(source, plan)
		usage = &index
	}
	selected := make(map[string]bool, len(usage.Modules))
	for _, module := range usage.Modules {
		selected[module.PluginID+"\x00"+module.ModuleID] = true
	}
	for _, item := range r.manager.Plugins() {
		if !item.Enabled || !hasDynamicReadPermission(item.Manifest.Permissions) {
			continue
		}
		for _, module := range item.Manifest.Modules {
			if !renderExecutableModule(module.Type) {
				continue
			}
			// A dynamic render module without declarative usage cannot be proven
			// irrelevant to this page, so keep the page on the live path.
			if len(module.Usage) == 0 || selected[item.Manifest.ID+"\x00"+module.ID] {
				return false
			}
		}
	}
	return true
}

func hasDynamicReadPermission(permissions []string) bool {
	for _, permission := range permissions {
		if slices.Contains([]string{"pages:read", "pages:content", "attachments:read", "settings:read", "storage:read"}, permission) {
			return true
		}
	}
	return false
}

func renderExecutableModule(moduleType string) bool {
	switch moduleType {
	case "renderer-extension", "code-highlighter", "content-substitution", "macro":
		return true
	default:
		return false
	}
}
