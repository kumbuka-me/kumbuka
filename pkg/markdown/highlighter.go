package markdown

import (
	"io"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/renderer"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/util"
)

// codeHighlighterExtension installs the active plugin's fenced-code renderer.
type codeHighlighterExtension struct {
	// owner stores the owner value used by code highlighter extension.
	owner string
	// module stores the module value used by code highlighter extension.
	module plugin.CodeHighlighterModule
	// context stores the context value used by code highlighter extension.
	context plugin.Context
}

// RendererOptions installs one code-block renderer for the active highlighter provider.
func (e codeHighlighterExtension) RendererOptions(_ *goldhtml.Config) []goldhtml.Option {
	return []goldhtml.Option{
		goldhtml.WithNodeRenderer(ast.KindCodeBlock, goldhtml.NodeRendererFunc(codeHighlighterRenderer(e).render)),
	}
}

// codeHighlighterRenderer delegates fenced code to the active plugin and owns plain fallback rendering.
type codeHighlighterRenderer codeHighlighterExtension

// render highlights a fenced code block or emits the normal escaped fallback.
func (r codeHighlighterRenderer) render(
	writer io.Writer,
	source []byte,
	node ast.Node,
	entering bool,
	renderContext renderer.Context,
) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	block := node.(*ast.CodeBlock)
	if block.CodeBlockKind != ast.CodeBlockKindFenced {
		return renderPlainCodeBlock(writer, source, block, renderContext)
	}

	language, _ := block.Language(source)
	code := codeBlockSource(block, source)

	result, err := plugin.Guard(r.owner, func() (plugin.CodeHighlightResult, error) {
		return r.module.Highlighter.Highlight(r.context, language, code)
	})
	if err != nil {
		return ast.WalkStop, err
	}
	if !result.Matched {
		return renderPlainCodeBlock(writer, source, block, renderContext)
	}

	w := writer.(util.BufWriter)
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

// codeBlockSource returns the literal source covered by a code block's value.
func codeBlockSource(block *ast.CodeBlock, source []byte) string {
	var output strings.Builder
	_, _ = block.Value.WriteTo(&output, source)
	return output.String()
}

// renderPlainCodeBlock preserves Goldmark-compatible code-block output when no provider matches.
func renderPlainCodeBlock(
	writer io.Writer,
	source []byte,
	block *ast.CodeBlock,
	renderContext renderer.Context,
) (ast.WalkStatus, error) {
	w := writer.(util.BufWriter)
	textWriter := goldhtml.ContextTextWriter(renderContext)

	if _, err := w.WriteString("<pre><code"); err != nil {
		return ast.WalkStop, err
	}
	if language, ok := block.Language(source); ok {
		if _, err := w.WriteString(` class="language-`); err != nil {
			return ast.WalkStop, err
		}
		if _, err := textWriter.WriteString(language); err != nil {
			return ast.WalkStop, err
		}
		if err := w.WriteByte('"'); err != nil {
			return ast.WalkStop, err
		}
	}
	if err := w.WriteByte('>'); err != nil {
		return ast.WalkStop, err
	}
	if _, err := block.Value.WriteTo(textWriter, source); err != nil {
		return ast.WalkStop, err
	}
	if _, err := w.WriteString("</code></pre>\n"); err != nil {
		return ast.WalkStop, err
	}

	return ast.WalkSkipChildren, nil
}
