package markdown

import (
	"html"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmrenderer "github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// codeHighlighterExtension installs the active plugin's fenced-code renderer.
type codeHighlighterExtension struct {
	owner   string
	module  plugin.CodeHighlighterModule
	context plugin.Context
}

// Extend installs one fenced-code renderer for the active highlighter provider.
func (e codeHighlighterExtension) Extend(markdown goldmark.Markdown) {
	markdown.Renderer().AddOptions(
		gmrenderer.WithNodeRenderers(
			util.Prioritized(codeHighlighterRenderer(e), 100),
		),
	)
}

// codeHighlighterRenderer delegates fenced code to the active plugin and owns plain fallback rendering.
type codeHighlighterRenderer codeHighlighterExtension

// RegisterFuncs implements renderer.NodeRenderer.
func (r codeHighlighterRenderer) RegisterFuncs(reg gmrenderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, r.render)
}

// render highlights a fenced code block or emits the normal escaped fallback.
func (r codeHighlighterRenderer) render(
	w util.BufWriter,
	source []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	block := node.(*ast.FencedCodeBlock)
	language := string(block.Language(source))
	code := fencedCodeSource(block, source)

	result, err := plugin.Guard(r.owner, func() (plugin.CodeHighlightResult, error) {
		return r.module.Highlighter.Highlight(r.context, language, code)
	})
	if err != nil {
		return ast.WalkStop, err
	}
	if !result.Matched {
		return renderPlainFencedCode(w, language, code)
	}

	if _, err := w.WriteString(`<div data-kumbuka-plugin="` + r.owner + `" data-kumbuka-module="` + r.module.ID + `">`); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.WriteString(result.HTML); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.WriteString("</div>\n"); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkSkipChildren, nil
}

// fencedCodeSource returns the literal source covered by a fenced block's line segments.
func fencedCodeSource(block *ast.FencedCodeBlock, source []byte) string {
	var output strings.Builder
	for index := 0; index < block.Lines().Len(); index++ {
		segment := block.Lines().At(index)
		_, _ = output.Write(segment.Value(source))
	}
	return output.String()
}

// renderPlainFencedCode preserves Goldmark-compatible fenced-code output when no provider matches.
func renderPlainFencedCode(w util.BufWriter, language, source string) (ast.WalkStatus, error) {
	if _, err := w.WriteString("<pre><code"); err != nil {
		return ast.WalkStop, err
	}
	if language != "" {
		if _, err := w.WriteString(` class="language-` + html.EscapeString(language) + `"`); err != nil {
			return ast.WalkStop, err
		}
	}
	if _, err := w.WriteString(">" + html.EscapeString(source) + "</code></pre>\n"); err != nil {
		return ast.WalkStop, err
	}
	return ast.WalkSkipChildren, nil
}
