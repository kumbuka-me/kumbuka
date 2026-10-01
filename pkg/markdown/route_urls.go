package markdown

import (
	"github.com/containeroo/httpprefix"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/text"
)

// routeURLTransformer applies the deployment boundary while Markdown URLs are still structured nodes.
type routeURLTransformer struct{ prefix string }

func (transformer routeURLTransformer) Transform(document *ast.Document, reader text.Reader, _ parser.Context) {
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch value := node.(type) {
		case *ast.Link:
			value.Destination = routedMarkdownDestination(value.Destination, reader, transformer.prefix)
		case *ast.Image:
			value.Destination = routedMarkdownDestination(value.Destination, reader, transformer.prefix)
		}
		return ast.WalkContinue, nil
	})
}

func routedMarkdownDestination(value text.SingleLineValue, reader text.Reader, prefix string) text.SingleLineValue {
	target := string(value.Bytes(reader.Source()))
	routed := httpprefix.RouteURL(prefix, target)
	if routed == target {
		return value
	}
	return text.NewSingleLineValueFromString(routed, text.IdentityDecoder)
}
