package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
)

// RenderSelector identifies one contribution and whether its execution can be limited to pages whose source usage index contains the module.
type RenderSelector struct {
	// PluginID identifies the plugin associated with render selector.
	PluginID string
	// ModuleID identifies the module associated with render selector.
	ModuleID string
	// UsageKey stores the usage key value used by render selector.
	UsageKey string
	// SourceAware reports whether source aware applies to render selector.
	SourceAware bool
}

// ContentPreprocessorBinding is one pre-sorted content preprocessor in a render plan.
type ContentPreprocessorBinding struct {
	// Selector stores the selector value used by content preprocessor binding.
	Selector RenderSelector
	// Module stores the module value used by content preprocessor binding.
	Module ContentPreprocessor
	// Priority stores the priority setting for content preprocessor binding.
	Priority int
	// Order stores the order setting for content preprocessor binding.
	Order int
}

// PreprocessorBinding is one source preprocessor in registry order.
type PreprocessorBinding struct {
	// Selector stores the selector value used by preprocessor binding.
	Selector RenderSelector
	// Module stores the module value used by preprocessor binding.
	Module Preprocessor
	// Order stores the order setting for preprocessor binding.
	Order int
}

// MarkdownExtensionBinding is one host Markdown extension in registry order.
type MarkdownExtensionBinding struct {
	// Selector stores the selector value used by markdown extension binding.
	Selector RenderSelector
	// Module stores the module value used by markdown extension binding.
	Module MarkdownExtension
}

// CodeHighlighterBinding is the single active code highlighter, when present.
type CodeHighlighterBinding struct {
	// Selector stores the selector value used by code highlighter binding.
	Selector RenderSelector
	// Module stores the module value used by code highlighter binding.
	Module CodeHighlighterModule
}

// MacroBinding is one uniquely named macro contribution.
type MacroBinding struct {
	// Selector stores the selector value used by macro binding.
	Selector RenderSelector
	// Module stores the module value used by macro binding.
	Module Macro
}

// PostprocessorBinding is one HTML postprocessor in registry order.
type PostprocessorBinding struct {
	// Selector stores the selector value used by postprocessor binding.
	Selector RenderSelector
	// Module stores the module value used by postprocessor binding.
	Module Postprocessor
}

// WidgetBinding is one widget contribution in deterministic registry order.
type WidgetBinding struct {
	// PluginID identifies the plugin associated with widget binding.
	PluginID string
	// ModuleID identifies the module associated with widget binding.
	ModuleID string
	// Surface stores the surface value used by widget binding.
	Surface string
	// Width stores the width setting for widget binding.
	Width string
	// Order stores the order setting for widget binding.
	Order int
	// Module stores the module value used by widget binding.
	Module Widget
}

// SourceUsageBinding is one immutable source selector set owned by a plugin module.
type SourceUsageBinding struct {
	// PluginID identifies the plugin associated with source usage binding.
	PluginID string
	// Usage stores the usage value used by source usage binding.
	Usage SourceUsage
}

// RenderPlan is the immutable render-only view of the active registry. It is rebuilt only when plugin lifecycle state changes and shared by all renders of that generation. Callers must not mutate its slices or maps.
type RenderPlan struct {
	// Generation stores the generation value used by render plan.
	Generation uint64
	// ContentPreprocessors contains the content preprocessors associated with render plan.
	ContentPreprocessors []ContentPreprocessorBinding
	// Preprocessors contains the preprocessors associated with render plan.
	Preprocessors []PreprocessorBinding
	// MarkdownExtensions contains the markdown extensions associated with render plan.
	MarkdownExtensions []MarkdownExtensionBinding
	// CodeHighlighter stores the code highlighter value used by render plan.
	CodeHighlighter *CodeHighlighterBinding
	// Macros maps keys to macros values used by render plan.
	Macros map[string]MacroBinding
	// Postprocessors contains the postprocessors associated with render plan.
	Postprocessors []PostprocessorBinding
	// Widgets contains the widgets associated with render plan.
	Widgets []WidgetBinding
	// RenderPolicies contains the render policies associated with render plan.
	RenderPolicies []RenderPolicy
	// SourceUsage contains the source usage associated with render plan.
	SourceUsage []SourceUsageBinding
	// UsageFingerprint stores the usage fingerprint value used by render plan.
	UsageFingerprint string

	// lifetimes contains the lifetimes associated with render plan.
	lifetimes []*lifetime
}

// renderPlanBuilder accumulates immutable bindings for one registry generation.
type renderPlanBuilder struct {
	// plan is the render plan being assembled.
	plan *RenderPlan
	// seenUsage tracks source-usage selectors already published to the plan.
	seenUsage map[string]bool
}

// buildRenderPlan constructs the immutable render view for one registry generation.
func buildRenderPlan(entries []Entry, generation uint64) *RenderPlan {
	builder := &renderPlanBuilder{
		plan: &RenderPlan{
			Generation: generation,
			Macros:     make(map[string]MacroBinding),
			lifetimes:  make([]*lifetime, 0, len(entries)),
		},
		seenUsage: make(map[string]bool),
	}

	for _, entry := range entries {
		builder.addEntry(entry)
	}

	return builder.finish()
}

// selector returns source-usage metadata for one contribution and records it once.
func (b *renderPlanBuilder) selector(pluginID string, candidate any) RenderSelector {
	selector := RenderSelector{PluginID: pluginID}
	provider, ok := candidate.(SourceUsageProvider)
	if !ok {
		return selector
	}

	usage := provider.SourceUsage()
	usage.Rules = slices.Clone(usage.Rules)
	selector.ModuleID = usage.ModuleID
	if usage.ModuleID != "" {
		selector.UsageKey = pluginID + "\x00" + usage.ModuleID
	}
	selector.SourceAware = usage.ModuleID != "" && len(usage.Rules) != 0
	if !selector.SourceAware || b.seenUsage[selector.UsageKey] {
		return selector
	}

	b.seenUsage[selector.UsageKey] = true
	b.plan.SourceUsage = append(b.plan.SourceUsage, SourceUsageBinding{PluginID: pluginID, Usage: usage})
	return selector
}

// addEntry appends every contribution from one active plugin entry.
func (b *renderPlanBuilder) addEntry(entry Entry) {
	pluginID := entry.Descriptor.ID
	b.plan.lifetimes = append(b.plan.lifetimes, entry.lifetime)

	for _, module := range entry.Contributions.ContentPreprocessors {
		b.plan.ContentPreprocessors = append(b.plan.ContentPreprocessors, ContentPreprocessorBinding{
			Selector: b.selector(pluginID, module),
			Module:   module,
			Priority: module.Priority(),
		})
	}
	for _, module := range entry.Contributions.Preprocessors {
		b.plan.Preprocessors = append(b.plan.Preprocessors, PreprocessorBinding{
			Selector: b.selector(pluginID, module),
			Module:   module,
			Order:    len(b.plan.Preprocessors),
		})
	}
	for _, module := range entry.Contributions.MarkdownExtensions {
		b.plan.MarkdownExtensions = append(b.plan.MarkdownExtensions, MarkdownExtensionBinding{
			Selector: b.selector(pluginID, module),
			Module:   module,
		})
	}
	for _, module := range entry.Contributions.CodeHighlighters {
		binding := CodeHighlighterBinding{
			Selector: b.selector(pluginID, module.Highlighter),
			Module:   module,
		}
		b.plan.CodeHighlighter = &binding
	}
	for _, module := range entry.Contributions.Macros {
		b.plan.Macros[module.Name()] = MacroBinding{
			Selector: b.selector(pluginID, module),
			Module:   module,
		}
	}
	for _, module := range entry.Contributions.Postprocessors {
		b.plan.Postprocessors = append(b.plan.Postprocessors, PostprocessorBinding{
			Selector: b.selector(pluginID, module),
			Module:   module,
		})
	}
	for _, module := range entry.Contributions.Widgets {
		b.plan.Widgets = append(b.plan.Widgets, WidgetBinding{
			PluginID: pluginID,
			ModuleID: module.ID,
			Surface:  module.Surface,
			Width:    module.Width,
			Order:    module.Order,
			Module:   module.Widget,
		})
	}
	b.plan.RenderPolicies = append(b.plan.RenderPolicies, entry.Contributions.RenderPolicies...)
}

// finish sorts order-sensitive bindings and computes the source-usage fingerprint.
func (b *renderPlanBuilder) finish() *RenderPlan {
	sort.SliceStable(b.plan.ContentPreprocessors, func(i, j int) bool {
		return b.plan.ContentPreprocessors[i].Priority < b.plan.ContentPreprocessors[j].Priority
	})
	for index := range b.plan.ContentPreprocessors {
		b.plan.ContentPreprocessors[index].Order = index
	}
	sort.SliceStable(b.plan.Widgets, func(i, j int) bool {
		return b.plan.Widgets[i].Order < b.plan.Widgets[j].Order
	})

	b.plan.UsageFingerprint = renderUsageFingerprint(b.plan.SourceUsage)
	return b.plan
}

// renderUsageFingerprint returns a stable digest for source-aware render selectors.
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

// publishEntriesLocked atomically publishes entries and their immutable render plan. The registry write lock must be held by the caller.
func (r *Registry) publishEntriesLocked(entries []Entry) {
	generation := r.renderGeneration + 1
	plan := buildRenderPlan(entries, generation)
	r.entries = entries
	r.renderGeneration = generation
	r.renderPlan = plan
}

// rebuildRenderPlanLocked initializes a missing plan without changing lifecycle generation. The registry write lock must be held by the caller.
func (r *Registry) rebuildRenderPlanLocked() {
	if r.renderGeneration == 0 {
		r.renderGeneration = 1
	}
	r.renderPlan = buildRenderPlan(r.entries, r.renderGeneration)
}
