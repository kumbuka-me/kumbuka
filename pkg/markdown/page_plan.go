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
	result := pageRenderPlan{}
	if plan == nil {
		return result
	}

	for _, binding := range plan.ContentPreprocessors {
		if selectorSelected(binding.Selector, usage) || selectorHasExportParameters(binding.Selector, exportParameters) {
			result.contentPreprocessors = append(result.contentPreprocessors, binding)
		}
	}
	for _, binding := range plan.Preprocessors {
		if selectorSelected(binding.Selector, usage) {
			result.preprocessors = append(result.preprocessors, binding)
		}
	}
	for _, binding := range plan.MarkdownExtensions {
		if selectorSelected(binding.Selector, usage) {
			result.markdownExtensions = append(result.markdownExtensions, binding)
		}
	}
	if plan.CodeHighlighter != nil && selectorSelected(plan.CodeHighlighter.Selector, usage) {
		result.codeHighlighter = plan.CodeHighlighter
	}
	for name, binding := range plan.Macros {
		if !selectorSelected(binding.Selector, usage) {
			continue
		}
		if result.macros == nil {
			result.macros = make(map[string]plugin.MacroBinding)
		}
		result.macros[name] = binding
	}
	for _, binding := range plan.Postprocessors {
		if selectorSelected(binding.Selector, usage) {
			result.postprocessors = append(result.postprocessors, binding)
		}
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
