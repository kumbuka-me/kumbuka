package markdown

import "github.com/kumbuka-me/kumbuka/pkg/plugin"

// pageRenderPlan is the request-local subset of the immutable global render plan selected by one Markdown source. It contains only modules that can affect that source, while the global plan remains available for conservative include-all paths such as opaque replacements.
type pageRenderPlan struct {
	// contentPreprocessors contains the content preprocessors associated with page render plan.
	contentPreprocessors []plugin.ContentPreprocessorBinding
	// preprocessors contains the preprocessors associated with page render plan.
	preprocessors []plugin.PreprocessorBinding
	// markdownExtensions contains the markdown extensions associated with page render plan.
	markdownExtensions []plugin.MarkdownExtensionBinding
	// codeHighlighter stores the code highlighter value used by page render plan.
	codeHighlighter *plugin.CodeHighlighterBinding
	// macros maps keys to macros values used by page render plan.
	macros map[string]plugin.MacroBinding
	// postprocessors contains the postprocessors associated with page render plan.
	postprocessors []plugin.PostprocessorBinding
}

// newPageRenderPlan builds a page-specific render plan from the active plugin plan and usage index.
func newPageRenderPlan(
	plan *plugin.RenderPlan,
	usage usageSet,
	exportParameters map[string]map[string]map[string]string,
) pageRenderPlan {
	if plan == nil {
		return pageRenderPlan{}
	}

	result := pageRenderPlan{
		contentPreprocessors: selectedRenderBindings(plan.ContentPreprocessors, usage, func(binding plugin.ContentPreprocessorBinding) plugin.RenderSelector { return binding.Selector }, func(binding plugin.ContentPreprocessorBinding) bool {
			return selectorHasExportParameters(binding.Selector, exportParameters)
		}),
		preprocessors:      selectedRenderBindings(plan.Preprocessors, usage, func(binding plugin.PreprocessorBinding) plugin.RenderSelector { return binding.Selector }, nil),
		markdownExtensions: selectedRenderBindings(plan.MarkdownExtensions, usage, func(binding plugin.MarkdownExtensionBinding) plugin.RenderSelector { return binding.Selector }, nil),
		postprocessors:     selectedRenderBindings(plan.Postprocessors, usage, func(binding plugin.PostprocessorBinding) plugin.RenderSelector { return binding.Selector }, nil),
		macros:             selectedMacros(plan.Macros, usage),
	}
	if plan.CodeHighlighter != nil && selectorSelected(plan.CodeHighlighter.Selector, usage) {
		result.codeHighlighter = plan.CodeHighlighter
	}
	return result
}

// selectedRenderBindings filters one binding collection by source usage and an optional extra selector.
func selectedRenderBindings[T any](bindings []T, usage usageSet, selector func(T) plugin.RenderSelector, extra func(T) bool) []T {
	result := make([]T, 0, len(bindings))
	for _, binding := range bindings {
		if selectorSelected(selector(binding), usage) || (extra != nil && extra(binding)) {
			result = append(result, binding)
		}
	}
	return result
}

// selectedMacros filters named macro bindings by source usage.
func selectedMacros(bindings map[string]plugin.MacroBinding, usage usageSet) map[string]plugin.MacroBinding {
	var result map[string]plugin.MacroBinding
	for name, binding := range bindings {
		if !selectorSelected(binding.Selector, usage) {
			continue
		}
		if result == nil {
			result = make(map[string]plugin.MacroBinding)
		}
		result[name] = binding
	}
	return result
}

// selectorSelected reports whether a render selector is active for the current page.
func selectorSelected(selector plugin.RenderSelector, usage usageSet) bool {
	return !selector.SourceAware || usage[selector.UsageKey]
}

// selectorHasExportParameters reports whether a selector depends on export parameters.
func selectorHasExportParameters(selector plugin.RenderSelector, parameters map[string]map[string]map[string]string) bool {
	return selector.ModuleID != "" && len(parameters[selector.PluginID][selector.ModuleID]) != 0
}
