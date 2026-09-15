package markdown

import (
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/icons"
	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/kumbuka/internal/plugincap"
	"github.com/kumbuka-me/kumbuka/internal/renderprofile"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// renderPipeline pins one immutable global render plan for the whole document,
// including nested Markdown and the variable-provenance rendering pass.
type renderPipeline struct {
	// context carries cancellation through the complete render.
	context context.Context
	// trace collects optional per-render diagnostics.
	trace *renderprofile.Trace
	// capabilities contains request-local host capabilities exposed to plugins.
	capabilities map[string]plugin.Capability
	// plan is the immutable render-only contribution set for this lifecycle generation.
	plan *plugin.RenderPlan
	// features contains presentation feature flags visible to plugin modules.
	features map[string]bool
	// macros contains request-local macro render bindings.
	macros map[string]plugin.MacroRenderer
	// exportParameters contains request-local plugin export overrides.
	exportParameters map[string]map[string]map[string]string
	// usageSource is the Markdown source represented by pagePlan.
	usageSource string
	// pagePlan contains only modules selected for usageSource.
	pagePlan pageRenderPlan
	// opaqueReplacements reports whether plugin substitutions may add parser/postprocessor syntax.
	opaqueReplacements bool
}

// macroInvocation records one deferred macro expansion and its owner.
type macroInvocation struct {
	// owner identifies the plugin that recognized this invocation.
	owner string
	// macro is the contribution that will render the deferred invocation.
	macro plugin.Macro
	// arguments contains serialized macro arguments.
	arguments plugin.Invocation
	// placeholder marks the invocation position in intermediate HTML.
	placeholder string
}

// newRenderPipeline binds one leased global render plan to request-local state.
func newRenderPipeline(plan *plugin.RenderPlan, features map[string]bool, functions Functions, source string, iconCatalog *icons.Catalog) *renderPipeline {
	capabilities := plugincap.Capabilities(nil, nil, iconCatalog)
	maps.Copy(capabilities, functions.Capabilities)
	exportParameters := cloneExportParameters(functions.ExportParameters)

	trace := renderprofile.FromContext(functions.Context)
	index := functions.PluginUsage
	if !currentUsageIndex(index, plan, source) {
		stop := trace.Measure("usage_analysis")
		derived := analyzeUsage(source, plan)
		stop()
		index = &derived
	}
	stop := trace.Measure("page_plan")
	usage := usageSetFromIndex(*index)
	pagePlan := newPageRenderPlan(plan, usage, exportParameters)
	stop()

	return &renderPipeline{
		context:          functions.Context,
		trace:            trace,
		capabilities:     capabilities,
		plan:             plan,
		features:         plan.RenderFeatures(features),
		macros:           maps.Clone(functions.Macros),
		exportParameters: exportParameters,
		usageSource:      source,
		pagePlan:         pagePlan,
	}
}

// moduleContext builds the request-local context passed to plugin contributions.
func (r *Renderer) moduleContext(resolve func(string) string, options Options) plugin.Context {
	return plugin.Context{
		Context:          options.pipeline.context,
		Capabilities:     maps.Clone(options.pipeline.capabilities),
		Features:         maps.Clone(options.pipeline.features),
		Macros:           maps.Clone(options.pipeline.macros),
		ExportParameters: cloneExportParameters(options.pipeline.exportParameters),
		RenderMarkdown: func(source string) (string, error) {
			raw, _, err := r.renderRawResolved(source, resolve, options)
			return raw, err
		},
	}
}

// prepareContent runs selected content preprocessors once in pre-sorted priority order.
// If one module transforms Markdown, later modules are reselected from the new source
// without rerunning modules whose priority/order has already passed.
func (p *renderPipeline) prepareContent(source string, ctx plugin.Context) (plugin.PreparedContent, error) {
	prepared := plugin.PreparedContent{Markdown: source}
	page := p.pagePlanForSource(prepared.Markdown)
	modules := page.contentPreprocessors

	for position := 0; position < len(modules); position++ {
		binding := modules[position]
		before := prepared.Markdown
		result, err := plugin.Guard(binding.Selector.PluginID, func() (plugin.PreparedContent, error) {
			return binding.Module.PreprocessContent(ctx, prepared.Markdown)
		})
		if err != nil {
			return plugin.PreparedContent{}, err
		}
		prepared.Markdown = result.Markdown
		prepared.Replacements = append(prepared.Replacements, result.Replacements...)
		prepared.Inspectors = append(prepared.Inspectors, result.Inspectors...)
		prepared.ExportFields = append(prepared.ExportFields, result.ExportFields...)
		if prepared.Markdown == before {
			continue
		}

		page = p.pagePlanForSource(prepared.Markdown)
		modules = page.contentPreprocessors
		position = firstContentPreprocessorAfter(modules, binding.Order) - 1
	}
	return prepared, nil
}

func firstContentPreprocessorAfter(modules []plugin.ContentPreprocessorBinding, order int) int {
	for index, binding := range modules {
		if binding.Order > order {
			return index
		}
	}
	return len(modules)
}

// cloneExportParameters deep-copies request-local export values before plugin callbacks receive them.
func cloneExportParameters(source map[string]map[string]map[string]string) map[string]map[string]map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]map[string]map[string]string, len(source))
	for pluginID, modules := range source {
		moduleCopy := make(map[string]map[string]string, len(modules))
		for moduleID, values := range modules {
			moduleCopy[moduleID] = maps.Clone(values)
		}
		result[pluginID] = moduleCopy
	}
	return result
}

// preprocess runs selected source preprocessors in registry order. A transform
// may activate later modules, so the page plan is refreshed only when source changes.
func (p *renderPipeline) preprocess(source string, ctx plugin.Context, page pageRenderPlan) (string, pageRenderPlan, error) {
	modules := page.preprocessors
	for position := 0; position < len(modules); position++ {
		binding := modules[position]
		before := source
		var err error
		source, err = plugin.Guard(binding.Selector.PluginID, func() (string, error) {
			return binding.Module.Preprocess(ctx, source)
		})
		if err != nil {
			return "", pageRenderPlan{}, err
		}
		if source == before {
			continue
		}

		page = p.pagePlanForSource(source)
		modules = page.preprocessors
		position = firstPreprocessorAfter(modules, binding.Order) - 1
	}
	return source, page, nil
}

func firstPreprocessorAfter(modules []plugin.PreprocessorBinding, order int) int {
	for index, binding := range modules {
		if binding.Order > order {
			return index
		}
	}
	return len(modules)
}

// extensions creates active Goldmark extensions for the current page plan.
func (p *renderPipeline) extensions(ctx plugin.Context, page pageRenderPlan, includeAll bool) ([]goldmark.Extender, error) {
	result := make([]goldmark.Extender, 0, len(page.markdownExtensions)+1)
	modules := page.markdownExtensions
	if includeAll {
		modules = p.plan.MarkdownExtensions
	}
	for _, binding := range modules {
		extender, err := plugin.Guard(binding.Selector.PluginID, func() (goldmark.Extender, error) {
			return binding.Module.Extension(ctx), nil
		})
		if err != nil {
			return nil, err
		}
		if extender == nil {
			return nil, fmt.Errorf("plugin %s returned a nil Markdown extension", binding.Selector.PluginID)
		}
		result = append(result, extender)
	}

	highlighter := page.codeHighlighter
	if includeAll {
		highlighter = p.plan.CodeHighlighter
	}
	if highlighter != nil {
		result = append(result, codeHighlighterExtension{
			owner:   highlighter.Selector.PluginID,
			module:  highlighter.Module,
			context: ctx,
		})
	}
	return result, nil
}

// postprocess runs selected plugin HTML postprocessors in registry order.
func (p *renderPipeline) postprocess(source string, ctx plugin.Context, page pageRenderPlan, includeAll bool) (string, error) {
	modules := page.postprocessors
	if includeAll {
		modules = p.plan.Postprocessors
	}
	for _, binding := range modules {
		var err error
		source, err = plugin.Guard(binding.Selector.PluginID, func() (string, error) {
			return binding.Module.Postprocess(ctx, source)
		})
		if err != nil {
			return "", err
		}
	}
	return source, nil
}

// preprocessMacros protects code using CommonMark's own parser, including long
// fences, blockquote/list fences, and indented code. Macro names are globally
// unique, so candidate lines dispatch directly through the page plan's name map.
func (p *renderPipeline) preprocessMacros(source string, ctx plugin.Context, page pageRenderPlan) (string, []macroInvocation, error) {
	lines := strings.Split(source, "\n")
	protected := codeLines(source)

	var invocations []macroInvocation
	nonce := rand.Text()

	for index, line := range lines {
		if protected[index] {
			continue
		}
		name, ok := macroInvocationName(line)
		if !ok {
			continue
		}
		binding, ok := page.macros[name]
		if !ok {
			continue
		}

		type parsed struct {
			arguments plugin.Invocation
			matched   bool
		}
		invocation, err := plugin.Guard(binding.Selector.PluginID, func() (parsed, error) {
			if conditional, ok := binding.Module.(plugin.ConditionalMacro); ok && !conditional.Available(ctx) {
				return parsed{}, nil
			}
			if contextual, ok := binding.Module.(plugin.ContextualMacro); ok {
				args, matched, err := contextual.ParseContext(ctx, line)
				return parsed{args, matched}, err
			}
			args, matched := binding.Module.Parse(line)
			return parsed{args, matched}, nil
		})
		if err != nil {
			return "", nil, err
		}
		if !invocation.matched {
			continue
		}

		placeholder := `<div data-kumbuka-macro="` + nonce + "-" + strconv.Itoa(len(invocations)) + `"></div>`
		invocations = append(invocations, macroInvocation{binding.Selector.PluginID, binding.Module, invocation.arguments, placeholder})
		lines[index] = placeholder
	}

	return strings.Join(lines, "\n"), invocations, nil
}

func macroInvocationName(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if len(line) < 5 || !strings.HasPrefix(line, "{{") || !strings.HasSuffix(line, "}}") {
		return "", false
	}
	body := strings.TrimSpace(line[2 : len(line)-2])
	if body == "" {
		return "", false
	}
	if end := strings.IndexAny(body, " \t"); end >= 0 {
		body = body[:end]
	}
	if body == "" || strings.ContainsAny(body, "{}\r\n") {
		return "", false
	}
	return body, true
}

// expandMacros renders deferred macros and replaces their placeholders in order.
func (p *renderPipeline) expandMacros(source string, invocations []macroInvocation, ctx plugin.Context) (string, error) {
	for _, invocation := range invocations {
		replacement, err := plugin.Guard(invocation.owner, func() (string, error) {
			return invocation.macro.Render(ctx, invocation.arguments)
		})
		if err != nil {
			return "", err
		}
		source = strings.Replace(source, invocation.placeholder, replacement, 1)
	}
	return source, nil
}

// codeLines records code body lines, not fences themselves, whose delimiters
// cannot match a standalone macro invocation.
func codeLines(source string) map[int]bool {
	document := goldmark.New().Parser().Parse(text.NewReader([]byte(source)))
	protected := make(map[int]bool)
	starts := []int{0}

	for index, char := range source {
		if char == '\n' {
			starts = append(starts, index+1)
		}
	}

	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || (node.Kind() != ast.KindFencedCodeBlock && node.Kind() != ast.KindCodeBlock) {
			return ast.WalkContinue, nil
		}
		for i := 0; i < node.Lines().Len(); i++ {
			segment := node.Lines().At(i)
			line := sort.Search(len(starts), func(i int) bool { return starts[i] > segment.Start }) - 1
			protected[line] = true
		}
		return ast.WalkSkipChildren, nil
	})

	return protected
}
