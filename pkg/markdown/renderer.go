package markdown

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/renderprofile"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	pluginmarkdown "github.com/kumbuka-me/sdk/markdown"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	goldhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
)

// Renderer converts Kumbuka Markdown into sanitized HTML.
type Renderer struct {
	// sanitizer removes unsafe HTML from rendered output.
	sanitizer     *bluemonday.Policy
	registry      *plugin.Registry
	manager       *plugin.Manager
	timingLogger  *slog.Logger
	artifactBuild string
	iconCatalog   *icons.Catalog
}

// Heading describes one rendered Markdown heading used in a page table of contents.
type Heading struct {
	// Level is the HTML heading level from 1 through 6.
	Level int
	// ID is the rendered heading anchor identifier.
	ID string
	// Title is the plain-text heading label.
	Title string
}

// RenderedPage contains sanitized page HTML and its extracted heading structure.
type RenderedPage struct {
	// HTML is the sanitized rendered Markdown.
	HTML string
	// Contents contains headings in document order.
	Contents []Heading
	// Inspectors contains plugin-owned reading-page metadata for substitutions used by this page.
	Inspectors []plugin.Inspector
	// ExportFields contains plugin-owned request-local export controls used by this page.
	ExportFields []plugin.ExportField
}

// Functions supplies request-local plugin capabilities and export data.
// Bindings cannot activate an unregistered macro.
type Functions struct {
	Capabilities     map[string]plugin.Capability
	Context          context.Context
	Macros           map[string]plugin.MacroRenderer
	ExportParameters map[string]map[string]map[string]string
	// PluginUsage is derived persisted metadata for saved pages. Nil requests transient analysis.
	PluginUsage *pluginusage.Index
}

// Close releases the attached plugin manager, if any. Renderers created with
// NewWithRegistry alone do not own plugin runtime resources.
func (r *Renderer) Close(ctx context.Context) error {
	if r.manager != nil {
		return r.manager.Close(ctx)
	}
	return nil
}

// NewWithRegistry uses an application-owned registry for every render path.
// An empty registry enables only the remaining core Markdown features.
func NewWithRegistry(registry *plugin.Registry) *Renderer {
	if registry == nil {
		registry = &plugin.Registry{}
	}

	return &Renderer{sanitizer: newSanitizer(), registry: registry, iconCatalog: icons.Builtin()}
}

// NewWithManager attaches a plugin manager to a renderer so
// lifecycle metadata and browser assets remain available through PluginManager.
// Closing the renderer closes the manager.
func NewWithManager(registry *plugin.Registry, manager *plugin.Manager) *Renderer {
	renderer := NewWithRegistry(registry)
	renderer.manager = manager
	renderer.iconCatalog = icons.NewCatalog(manager)
	return renderer
}

// IconCatalog returns the icon catalog bound to this renderer plugin lifecycle.
func (r *Renderer) IconCatalog() *icons.Catalog { return r.iconCatalog }

// engine constructs a Goldmark renderer from administrator-controlled options.
func engine(contributed []goldmark.Extender, annotationRanges []annotationRange) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(contributed...),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(
				util.Prioritized(imageWidthTransformer{}, 100),
				util.Prioritized(annotationTransformer{ranges: annotationRanges}, 210),
			),
		),
		goldmark.WithRendererOptions(
			goldhtml.WithUnsafe(),
			renderer.WithNodeRenderers(
				util.Prioritized(annotationNodeRenderer{ranges: annotationRanges}, 110),
			),
		),
	)
}

// Links extracts unique canonical wiki-link targets from Markdown source.
func Links(source string) []string {
	seen := make(map[string]bool)
	var links []string

	walkWikiLinks(source, func(target, _ string) {
		pageTarget, _ := SplitHeadingTarget(target)
		slug := Slug(pageTarget)
		if slug == "" || seen[slug] {
			return
		}

		seen[slug] = true
		links = append(links, slug)
	})
	return links
}

// Render converts Markdown into sanitized HTML using default rendering options.
func (r *Renderer) Render(source string) (string, error) {
	return r.RenderResolvedWithOptions(source, Slug, DefaultOptions())
}

// RenderResolved converts Markdown into sanitized HTML using default rendering options and a custom wiki-link resolver.
func (r *Renderer) RenderResolved(
	source string,
	resolve func(string) string,
) (string, error) {
	return r.RenderResolvedWithOptions(source, resolve, DefaultOptions())
}

// RenderResolvedWithOptions converts Markdown using administrator-controlled rendering options.
func (r *Renderer) RenderResolvedWithOptions(
	source string,
	resolve func(string) string,
	options Options,
) (string, error) {
	rendered, err := r.RenderPageResolvedWithOptions(
		source,
		resolve,
		options,
	)
	if err != nil {
		return "", err
	}

	return rendered.HTML, nil
}

// RenderPageResolved renders Markdown using default options and returns both HTML and page contents.
func (r *Renderer) RenderPageResolved(
	source string,
	resolve func(string) string,
) (RenderedPage, error) {
	return r.RenderPageResolvedWithOptions(
		source,
		resolve,
		DefaultOptions(),
	)
}

// RenderPageResolvedWithOptions renders Markdown and returns both sanitized HTML and page contents.
func (r *Renderer) RenderPageResolvedWithOptions(
	source string,
	resolve func(string) string,
	options Options,
) (RenderedPage, error) {
	return r.RenderPageResolvedWithFunctions(
		source,
		resolve,
		options,
		Functions{},
	)
}

// RenderPageResolvedWithFunctions renders Markdown and expands trusted dynamic page functions.
func (r *Renderer) RenderPageResolvedWithFunctions(
	source string,
	resolve func(string) string,
	options Options,
	functions Functions,
) (rendered RenderedPage, err error) {
	sourceBytes := len(source)
	execution := functions.Context
	if execution == nil {
		execution = context.Background()
	}
	execution, cancel := context.WithTimeout(execution, 30*time.Second)
	defer cancel()

	var trace *renderprofile.Trace
	if r.timingLogger != nil {
		trace = renderprofile.New()
		execution = renderprofile.WithContext(execution, trace)
		defer func() {
			r.logRenderTimings(trace, sourceBytes, len(rendered.HTML), err)
		}()
	}
	functions.Context = execution

	stop := trace.Measure("render_plan_acquire")
	plan, release := r.registry.AcquireRenderPlan()
	stop()
	defer release()

	stop = trace.Measure("pipeline_setup")
	options.pipeline = newRenderPipeline(plan, r.pluginFeatures(), functions, source, r.iconCatalog)
	stop()

	stop = trace.Measure("content_preprocess")
	prepared, err := options.pipeline.prepareContent(source, r.moduleContext(resolve, options))
	stop()
	if err != nil {
		return RenderedPage{}, err
	}
	source = prepared.Markdown
	options.pipeline.setUsageSource(source)
	options.pipeline.opaqueReplacements = len(prepared.Replacements) != 0
	options.annotations = prepared.Replacements

	if len(prepared.Replacements) != 0 {
		stop = trace.Measure("render_with_annotations")
		rendered, err = r.renderPageWithAnnotations(source, resolve, options)
		stop()
	} else {
		stop = trace.Measure("render_page")
		rendered, err = r.renderPage(source, resolve, options)
		stop()
	}
	if err != nil {
		return RenderedPage{}, err
	}
	rendered.Inspectors = prepared.Inspectors
	rendered.ExportFields = prepared.ExportFields
	return rendered, nil
}

// renderPageWithAnnotations restores plugin substitutions and adds safe origin wrappers when semantics stay unchanged.
func (r *Renderer) renderPageWithAnnotations(
	source string,
	resolve func(string) string,
	options Options,
) (RenderedPage, error) {
	stop := options.pipeline.trace.Measure("annotation_resolve_plain")
	plain, _ := resolvePluginReplacements(source, options.annotations)
	stop()
	plainOptions := options
	plainOptions.annotations = nil
	stop = options.pipeline.trace.Measure("annotation_plain_render")
	normal, err := r.renderPage(plain, resolve, plainOptions)
	stop()
	if err != nil {
		return RenderedPage{}, err
	}
	stop = options.pipeline.trace.Measure("annotation_wrapped_render")
	annotated, err := r.renderPage(source, resolve, options)
	stop()
	if err != nil {
		return normal, nil
	}
	if equivalentAnnotationHTML(annotated.HTML, normal.HTML) {
		normal.HTML = annotated.HTML
	}
	return normal, nil
}

// renderPage renders one Markdown page with optional dynamic functions and heading extraction.
func (r *Renderer) renderPage(
	source string,
	resolve func(string) string,
	options Options,
) (RenderedPage, error) {
	pagePlan := options.pipeline.pagePlanForSource(source)
	stop := options.pipeline.trace.Measure("macro_preprocess")
	source, invocations, err := options.pipeline.preprocessMacros(source, r.moduleContext(resolve, options), pagePlan)
	stop()
	if err != nil {
		return RenderedPage{}, err
	}
	stop = options.pipeline.trace.Measure("markdown_render")
	raw, renderPlan, err := r.renderRawResolved(source, resolve, options)
	stop()
	if err != nil {
		return RenderedPage{}, err
	}
	// Preserve the established contents list: generated macro headings are not
	// part of the source page's navigation.
	stop = options.pipeline.trace.Measure("heading_sanitize")
	headingHTML := r.sanitizer.Sanitize(raw)
	stop()
	stop = options.pipeline.trace.Measure("heading_extract")
	contents := extractHeadings(headingHTML)
	stop()
	stop = options.pipeline.trace.Measure("macro_expand")
	raw, err = options.pipeline.expandMacros(raw, invocations, r.moduleContext(resolve, options))
	stop()
	if err != nil {
		return RenderedPage{}, err
	}
	stop = options.pipeline.trace.Measure("postprocess")
	raw, err = options.pipeline.postprocess(
		raw,
		r.moduleContext(resolve, options),
		renderPlan,
		len(invocations) != 0 || options.pipeline.opaqueReplacements,
	)
	stop()
	if err != nil {
		return RenderedPage{}, err
	}
	stop = options.pipeline.trace.Measure("final_sanitize")
	html := r.sanitizer.Sanitize(raw)
	stop()
	return RenderedPage{HTML: html, Contents: contents}, nil
}

// renderRawResolved renders Markdown extensions into unsanitized HTML for recursive block rendering.
func (r *Renderer) renderRawResolved(
	source string,
	resolve func(string) string,
	options Options,
) (string, pageRenderPlan, error) {
	if err := options.pipeline.context.Err(); err != nil {
		return "", pageRenderPlan{}, err
	}
	if options.depth >= 64 {
		return "", pageRenderPlan{}, errors.New("markdown nesting limit exceeded")
	}
	options.depth++

	ctx := r.moduleContext(resolve, options)
	pagePlan := options.pipeline.pagePlanForSource(source)
	var err error
	stop := options.pipeline.trace.Measure("markdown_preprocess")
	source, pagePlan, err = options.pipeline.preprocess(source, ctx, pagePlan)
	stop()
	if err != nil {
		return "", pageRenderPlan{}, err
	}

	if options.WikiLinks {
		stop = options.pipeline.trace.Measure("wiki_links")
		source = rewriteWikiLinks(
			source,
			resolve,
			wikiLinkPrefix(options),
		)
		stop()
	}

	stop = options.pipeline.trace.Measure("replacement_resolve")
	source, annotationRanges := resolvePluginReplacements(source, options.annotations)
	stop()

	var output bytes.Buffer

	stop = options.pipeline.trace.Measure("extensions")
	extensions, err := options.pipeline.extensions(ctx, pagePlan, options.pipeline.opaqueReplacements)
	stop()
	if err != nil {
		return "", pageRenderPlan{}, err
	}
	// Conversion invokes contributed parsers, transformers, and node renderers.
	stop = options.pipeline.trace.Measure("goldmark")
	_, err = plugin.Guard("Markdown conversion", func() (struct{}, error) {
		return struct{}{}, engine(extensions, annotationRanges).Convert([]byte(source), &output)
	})
	stop()
	if err != nil {
		return "", pageRenderPlan{}, err
	}

	raw := output.String()

	return raw, pagePlan, nil
}

// fenceDelimiter returns the Markdown fence marker when a line starts a fenced code block.
func fenceDelimiter(line string) string { return pluginmarkdown.Fence(line) }

// walkWikiLinks visits wiki links outside fenced code blocks in source order.
func walkWikiLinks(
	source string,
	visit func(target, label string),
) {
	fence := ""

	for line := range strings.SplitSeq(source, "\n") {
		marker := fenceDelimiter(line)

		if fence != "" {
			if pluginmarkdown.Closes(line, fence) {
				fence = ""
			}

			continue
		}

		if marker != "" {
			fence = marker
			continue
		}

		walkWikiLinksLine(line, visit)
	}
}

// walkWikiLinksLine visits syntactically valid wiki links in one Markdown line.
func walkWikiLinksLine(
	line string,
	visit func(target, label string),
) {
	for offset := 0; offset < len(line); {
		start := strings.Index(line[offset:], "[[")

		if start < 0 {
			return
		}

		start += offset

		if start > 0 && line[start-1] == '\\' {
			offset = start + 2
			continue
		}

		end := strings.Index(line[start+2:], "]]")
		if end < 0 {
			return
		}

		end += start + 2

		target, label, ok := parseWikiLink(
			line[start+2 : end],
		)

		if ok {
			visit(target, label)
		}

		offset = end + 2
	}
}

// parseWikiLink splits a wiki-link body into target and optional label.
func parseWikiLink(
	value string,
) (target string, label string, ok bool) {
	target, label, hasLabel := strings.Cut(value, "|")
	target = strings.TrimSpace(target)

	if target == "" {
		return "", "", false
	}

	label = strings.TrimSpace(label)

	if !hasLabel || label == "" {
		label = target
	}

	return target, label, true
}

// SplitHeadingTarget separates a wiki page target from an optional heading fragment.
func SplitHeadingTarget(target string) (page string, heading string) {
	page, heading, _ = strings.Cut(strings.TrimSpace(target), "#")
	return strings.TrimSpace(page), strings.TrimSpace(heading)
}

// HeadingID converts a human-readable heading reference into Kumbuka's heading anchor form.
func HeadingID(value string) string {
	return Slug(strings.ReplaceAll(value, "/", " "))
}

// wikiLinkPrefix returns the configured wiki-link URL prefix.
func wikiLinkPrefix(options Options) string {
	if strings.TrimSpace(options.WikiLinkPrefix) == "" {
		return "/pages/"
	}

	return options.WikiLinkPrefix
}

// rewriteWikiLinks converts wiki-link syntax outside fenced code blocks into Markdown links.
func rewriteWikiLinks(
	source string,
	resolve func(string) string,
	prefix string,
) string {
	lines := strings.Split(source, "\n")
	fence := ""

	for index, line := range lines {
		marker := fenceDelimiter(line)

		if fence != "" {
			if pluginmarkdown.Closes(line, fence) {
				fence = ""
			}

			continue
		}

		if marker != "" {
			fence = marker
			continue
		}

		lines[index] = rewriteWikiLinksLine(
			line,
			resolve,
			prefix,
		)
	}

	return strings.Join(lines, "\n")
}

// rewriteWikiLinksLine converts wiki links in one Markdown line without regular expressions.
func rewriteWikiLinksLine(
	line string,
	resolve func(string) string,
	prefix string,
) string {
	var output strings.Builder

	offset := 0

	for offset < len(line) {
		start := strings.Index(line[offset:], "[[")

		if start < 0 {
			output.WriteString(line[offset:])
			break
		}

		start += offset

		if start > 0 && line[start-1] == '\\' {
			output.WriteString(line[offset : start+2])

			offset = start + 2
			continue
		}

		end := strings.Index(line[start+2:], "]]")
		if end < 0 {
			output.WriteString(line[offset:])
			break
		}

		end += start + 2

		target, label, ok := parseWikiLink(
			line[start+2 : end],
		)

		if !ok {
			output.WriteString(line[offset : end+2])

			offset = end + 2
			continue
		}

		output.WriteString(line[offset:start])
		output.WriteByte('[')
		output.WriteString(label)
		output.WriteString("](")
		pageTarget, heading := SplitHeadingTarget(target)
		resolved := resolve(pageTarget)
		output.WriteString(prefix)
		output.WriteString(resolved)
		if heading != "" {
			output.WriteByte('#')
			output.WriteString(HeadingID(heading))
		}
		output.WriteByte(')')

		offset = end + 2
	}

	return output.String()
}

// htmlHeadingLevel returns the numeric level of an h1-h6 element.
func htmlHeadingLevel(
	node *xhtml.Node,
) (level int, ok bool) {
	if node.Type != xhtml.ElementNode ||
		len(node.Data) != 2 {
		return 0, false
	}

	if node.Data[0] != 'h' {
		return 0, false
	}

	digit := node.Data[1]

	if digit < '1' || digit > '6' {
		return 0, false
	}

	return int(digit - '0'), true
}

// extractHeadings extracts rendered heading IDs and labels for page navigation.
func extractHeadings(rendered string) []Heading {
	document, err := xhtml.Parse(
		strings.NewReader(rendered),
	)
	if err != nil {
		return nil
	}

	var contents []Heading

	var walk func(*xhtml.Node)

	walk = func(node *xhtml.Node) {
		if level, ok := htmlHeadingLevel(node); ok {
			id := htmlAttribute(node, "id")

			if id != "" {
				contents = append(
					contents,
					Heading{
						Level: level,
						ID:    id,
						Title: strings.TrimSpace(
							htmlText(node),
						),
					},
				)
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(document)

	return contents
}

// htmlAttribute returns one HTML node attribute by key.
func htmlAttribute(
	node *xhtml.Node,
	key string,
) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return attribute.Val
		}
	}

	return ""
}

// htmlText returns the concatenated text content below an HTML node.
func htmlText(node *xhtml.Node) string {
	var output strings.Builder

	var walk func(*xhtml.Node)

	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			output.WriteString(current.Data)
		}

		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}

	walk(node)

	return output.String()
}
