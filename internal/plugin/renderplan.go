package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
)

// RenderSelector identifies one contribution and whether its execution can be
// limited to pages whose source usage index contains the module.
type RenderSelector struct {
	PluginID    string
	ModuleID    string
	UsageKey    string
	SourceAware bool
}

// ContentPreprocessorBinding is one pre-sorted content preprocessor in a render plan.
type ContentPreprocessorBinding struct {
	Selector RenderSelector
	Module   ContentPreprocessor
	Priority int
	Order    int
}

// PreprocessorBinding is one source preprocessor in registry order.
type PreprocessorBinding struct {
	Selector RenderSelector
	Module   Preprocessor
	Order    int
}

// MarkdownExtensionBinding is one host Markdown extension in registry order.
type MarkdownExtensionBinding struct {
	Selector RenderSelector
	Module   MarkdownExtension
}

// CodeHighlighterBinding is the single active code highlighter, when present.
type CodeHighlighterBinding struct {
	Selector RenderSelector
	Module   CodeHighlighterModule
}

// MacroBinding is one uniquely named macro contribution.
type MacroBinding struct {
	Selector RenderSelector
	Module   Macro
}

// PostprocessorBinding is one HTML postprocessor in registry order.
type PostprocessorBinding struct {
	Selector RenderSelector
	Module   Postprocessor
}

// WidgetBinding is one widget contribution in deterministic registry order.
type WidgetBinding struct {
	PluginID string
	ModuleID string
	Surface  string
	Module   Widget
}

// SourceUsageBinding is one immutable source selector set owned by a plugin module.
type SourceUsageBinding struct {
	PluginID string
	Usage    SourceUsage
}

// RenderPlan is the immutable render-only view of the active registry. It is
// rebuilt only when plugin lifecycle state changes and shared by all renders of
// that generation. Callers must not mutate its slices or maps.
type RenderPlan struct {
	Generation           uint64
	ContentPreprocessors []ContentPreprocessorBinding
	Preprocessors        []PreprocessorBinding
	MarkdownExtensions   []MarkdownExtensionBinding
	CodeHighlighter      *CodeHighlighterBinding
	Macros               map[string]MacroBinding
	Postprocessors       []PostprocessorBinding
	Widgets              []WidgetBinding
	RenderPolicies       []RenderPolicy
	SourceUsage          []SourceUsageBinding
	UsageFingerprint     string

	lifetimes []*lifetime
}

func buildRenderPlan(entries []Entry, generation uint64) *RenderPlan {
	plan := &RenderPlan{
		Generation: generation,
		Macros:     make(map[string]MacroBinding),
		lifetimes:  make([]*lifetime, 0, len(entries)),
	}
	seenUsage := make(map[string]bool)

	selector := func(pluginID string, candidate any) RenderSelector {
		result := RenderSelector{PluginID: pluginID}
		provider, ok := candidate.(SourceUsageProvider)
		if !ok {
			return result
		}
		usage := provider.SourceUsage()
		usage.Rules = slices.Clone(usage.Rules)
		result.ModuleID = usage.ModuleID
		if usage.ModuleID != "" {
			result.UsageKey = pluginID + "\x00" + usage.ModuleID
		}
		result.SourceAware = usage.ModuleID != "" && len(usage.Rules) != 0
		if !result.SourceAware {
			return result
		}
		if !seenUsage[result.UsageKey] {
			seenUsage[result.UsageKey] = true
			plan.SourceUsage = append(plan.SourceUsage, SourceUsageBinding{PluginID: pluginID, Usage: usage})
		}
		return result
	}

	for _, entry := range entries {
		pluginID := entry.Descriptor.ID
		plan.lifetimes = append(plan.lifetimes, entry.lifetime)

		for _, module := range entry.Contributions.ContentPreprocessors {
			plan.ContentPreprocessors = append(plan.ContentPreprocessors, ContentPreprocessorBinding{
				Selector: selector(pluginID, module),
				Module:   module,
				Priority: module.Priority(),
			})
		}
		for _, module := range entry.Contributions.Preprocessors {
			plan.Preprocessors = append(plan.Preprocessors, PreprocessorBinding{
				Selector: selector(pluginID, module),
				Module:   module,
				Order:    len(plan.Preprocessors),
			})
		}
		for _, module := range entry.Contributions.MarkdownExtensions {
			plan.MarkdownExtensions = append(plan.MarkdownExtensions, MarkdownExtensionBinding{
				Selector: selector(pluginID, module),
				Module:   module,
			})
		}
		for _, module := range entry.Contributions.CodeHighlighters {
			binding := CodeHighlighterBinding{
				Selector: selector(pluginID, module.Highlighter),
				Module:   module,
			}
			plan.CodeHighlighter = &binding
		}
		for _, module := range entry.Contributions.Macros {
			plan.Macros[module.Name()] = MacroBinding{
				Selector: selector(pluginID, module),
				Module:   module,
			}
		}
		for _, module := range entry.Contributions.Postprocessors {
			plan.Postprocessors = append(plan.Postprocessors, PostprocessorBinding{
				Selector: selector(pluginID, module),
				Module:   module,
			})
		}
		for _, module := range entry.Contributions.Widgets {
			plan.Widgets = append(plan.Widgets, WidgetBinding{
				PluginID: pluginID, ModuleID: module.ID, Surface: module.Surface, Module: module.Widget,
			})
		}
		plan.RenderPolicies = append(plan.RenderPolicies, entry.Contributions.RenderPolicies...)
	}

	sort.SliceStable(plan.ContentPreprocessors, func(i, j int) bool {
		return plan.ContentPreprocessors[i].Priority < plan.ContentPreprocessors[j].Priority
	})
	for index := range plan.ContentPreprocessors {
		plan.ContentPreprocessors[index].Order = index
	}
	plan.UsageFingerprint = renderUsageFingerprint(plan.SourceUsage)
	return plan
}

func renderUsageFingerprint(descriptors []SourceUsageBinding) string {
	hash := sha256.New()
	for _, descriptor := range descriptors {
		hash.Write([]byte(descriptor.PluginID))
		hash.Write([]byte{0})
		hash.Write([]byte(descriptor.Usage.ModuleID))
		hash.Write([]byte{0})
		for _, rule := range descriptor.Usage.Rules {
			hash.Write([]byte(rule.Contains))
			hash.Write([]byte{0})
			hash.Write([]byte(rule.Fence))
			hash.Write([]byte{0})
			hash.Write([]byte(rule.Macro))
			hash.Write([]byte{0})
			hash.Write([]byte(rule.Substitution))
			hash.Write([]byte{0xff})
		}
		hash.Write([]byte{0xfe})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// publishEntriesLocked atomically publishes entries and their immutable render
// plan. The registry write lock must be held by the caller.
func (r *Registry) publishEntriesLocked(entries []Entry) {
	generation := r.renderGeneration + 1
	plan := buildRenderPlan(entries, generation)
	r.entries = entries
	r.renderGeneration = generation
	r.renderPlan = plan
}

// rebuildRenderPlanLocked initializes a missing plan without changing lifecycle
// generation. The registry write lock must be held by the caller.
func (r *Registry) rebuildRenderPlanLocked() {
	if r.renderGeneration == 0 {
		r.renderGeneration = 1
	}
	r.renderPlan = buildRenderPlan(r.entries, r.renderGeneration)
}
