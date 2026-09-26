package markdown

import (
	"io"

	"github.com/kumbuka-me/kumbuka/pkg/mention"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
	"github.com/yuin/goldmark/v2/text"
	"github.com/yuin/goldmark/v2/util"
)

var kindUserMention = ast.NewNodeKind("KumbukaUserMention")

// userMentionNode wraps one authored @username reference.
type userMentionNode struct {
	ast.BaseInline
}

// newUserMentionNode creates one initialized mention node.
func newUserMentionNode() *userMentionNode {
	node := &userMentionNode{}
	node.Init(node)
	return node
}

// Kind returns the Goldmark node kind for Kumbuka mentions.
func (n *userMentionNode) Kind() ast.NodeKind { return kindUserMention }

// Dump returns a diagnostic representation of a mention node.
func (n *userMentionNode) Dump(_ []byte) *ast.NodeDump { return ast.NewNodeDump(n, nil) }

// mentionTransformer wraps authored mention ranges while leaving code spans and
// image alt text literal.
type mentionTransformer struct {
	ranges []mention.Range
}

// Transform adds mention nodes to ordinary Markdown text.
func (m mentionTransformer) Transform(document *ast.Document, reader text.Reader, _ parser.Context) {
	if len(m.ranges) == 0 {
		return
	}

	var nodes []*ast.Text
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node.Kind() {
		case ast.KindImage, ast.KindCodeSpan:
			return ast.WalkSkipChildren, nil
		}
		if textNode, ok := node.(*ast.Text); ok {
			nodes = append(nodes, textNode)
		}
		return ast.WalkContinue, nil
	})

	for _, node := range nodes {
		m.wrapText(node, reader.Decoder())
	}
}

// wrapText replaces source segments that intersect mention ranges with mention nodes.
func (m mentionTransformer) wrapText(node *ast.Text, decoder text.Decoder) {
	if node.Value.IsOwned() {
		return
	}
	index := node.Value.Index()
	start, stop := index.Start, index.Stop
	if start >= stop {
		return
	}

	parent := node.Parent()
	position := start
	changed := false
	appendText := func(from, to int, mention bool) {
		if from >= to {
			return
		}
		part := ast.NewText(text.NewSingleLineValueFromIndex(text.NewIndex(from, to), decoder))
		if to == stop {
			part.SetSoftLineBreak(node.SoftLineBreak())
			part.SetHardLineBreak(node.HardLineBreak())
		}
		if !mention {
			parent.InsertBefore(node, part)
			return
		}
		wrapper := newUserMentionNode()
		wrapper.AppendChild(part)
		parent.InsertBefore(node, wrapper)
	}

	for _, item := range m.ranges {
		from, to := max(position, item.Start), min(stop, item.End)
		if from >= to {
			continue
		}
		appendText(position, from, false)
		appendText(from, to, true)
		position = to
		changed = true
	}
	if !changed {
		return
	}
	appendText(position, stop, false)
	parent.RemoveChild(node)
}

// mentionHTMLRendererExtension renders mention nodes as a compact semantic span.
type mentionHTMLRendererExtension struct{}

// RendererOptions registers the mention HTML renderer with Goldmark.
func (mentionHTMLRendererExtension) RendererOptions(_ *goldhtml.Config) []goldhtml.Option {
	return []goldhtml.Option{
		goldhtml.WithNodeRenderer(kindUserMention, goldhtml.NodeRendererFunc(renderUserMention)),
	}
}

// renderUserMention writes the wrapper around one authored mention.
func renderUserMention(
	writer io.Writer,
	_ []byte,
	_ ast.Node,
	entering bool,
	_ renderer.Context,
) (ast.WalkStatus, error) {
	w := writer.(util.BufWriter)
	if entering {
		_, _ = w.WriteString(`<span class="user-mention" data-kumbuka-mention>`)
	} else {
		_, _ = w.WriteString("</span>")
	}
	return ast.WalkContinue, nil
}
