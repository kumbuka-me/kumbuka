package markdown

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

const maxImageWidthPixels = 10000

// imageWidthTransformer consumes a width directive immediately after a parsed
// Markdown image. Goldmark still owns image URLs, titles, alt text and references;
// code spans, code blocks and raw HTML are not interpreted as image syntax.
type imageWidthTransformer struct{}

// Transform applies image widths without changing the stored Markdown source.
func (imageWidthTransformer) Transform(document *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		image, ok := node.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}

		// Do not descend into alt text: nested images there are rendered as text.
		next, ok := image.NextSibling().(*ast.Text)
		if !ok {
			return ast.WalkSkipChildren, nil
		}
		width, consumed := parseImageWidthDirective(next.Segment.Value(source))
		if consumed == 0 {
			return ast.WalkSkipChildren, nil
		}

		image.SetAttributeString("style", "width:"+width)
		next.Segment.Start += consumed
		// An empty text node may still carry a Markdown line break.
		if next.Segment.Start == next.Segment.Stop && !next.SoftLineBreak() && !next.HardLineBreak() {
			next.Parent().RemoveChild(next.Parent(), next)
		}
		return ast.WalkSkipChildren, nil
	})
}

// parseImageWidthDirective accepts only {width=N}, {width=Npx} or {width=N%}.
// Invalid directives remain visible text instead of silently discarding input.
func parseImageWidthDirective(value []byte) (width string, consumed int) {
	const prefix = "{width="
	value, ok := bytes.CutPrefix(value, []byte(prefix))
	if !ok {
		return "", 0
	}

	end := bytes.IndexByte(value, '}')
	if end < 0 {
		return "", 0
	}

	width = normalizeImageWidth(string(value[:end]))
	if width == "" {
		return "", 0
	}

	return width, len(prefix) + end + 1
}

// normalizeImageWidth returns a bounded CSS width, using pixels for bare numbers.
// Only whole positive numbers are accepted; arbitrary CSS is never passed through.
func normalizeImageWidth(value string) string {
	unit, maximum := "px", maxImageWidthPixels
	if number, ok := strings.CutSuffix(value, "%"); ok {
		unit, maximum = "%", 100
		value = number
	} else {
		value, _ = strings.CutSuffix(value, "px")
	}
	if value == "" {
		return ""
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return ""
		}
	}
	width, err := strconv.Atoi(value)
	if err != nil || width < 1 || width > maximum {
		return ""
	}
	return strconv.Itoa(width) + unit
}

// validImageWidthStyle restricts the sanitizer to canonical, unit-bearing widths.
func validImageWidthStyle(value string) bool {
	return value != "" && normalizeImageWidth(value) == value
}
