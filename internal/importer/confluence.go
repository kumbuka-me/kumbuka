package importer

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	xhtml "golang.org/x/net/html"
)

// htmlMarkdownWriter converts the supported Confluence HTML subset into Markdown.
type htmlMarkdownWriter struct {
	// output accumulates Markdown emitted while walking the HTML tree.
	output strings.Builder
}

// importConfluenceFile converts one standalone Confluence HTML export into a page candidate.
func importConfluenceFile(name, extension string, data []byte) ([]Candidate, error) {
	if extension != ".html" && extension != ".htm" {
		return nil, newValidationError(
			"Imports from Confluence require .html, .htm, or .zip files.",
			errors.New("validate Confluence import: unsupported file type"),
		)
	}

	return confluenceCandidate(name, extension, data, "Confluence HTML")
}

// importConfluenceArchiveEntry converts one Confluence HTML archive entry into a page candidate.
func importConfluenceArchiveEntry(name, extension string, data []byte) ([]Candidate, error) {
	return confluenceCandidate(name, extension, data, "Confluence ZIP")
}

// confluenceCandidate converts HTML and discovers the page title required by the importer.
func confluenceCandidate(name, extension string, data []byte, source string) ([]Candidate, error) {
	markdown, err := htmlToMarkdown(data)
	if err != nil {
		return nil, err
	}

	title, err := markdownTitle(markdown)
	if err != nil {
		return nil, err
	}

	return []Candidate{{
		Slug:     strings.TrimSuffix(name, extension),
		Title:    title,
		Markdown: markdown,
		Source:   source,
	}}, nil
}

// htmlToMarkdown converts the supported Confluence HTML subset to Markdown.
func htmlToMarkdown(data []byte) (string, error) {
	root, err := xhtml.Parse(bytes.NewReader(data))
	if err != nil {
		return "", newValidationError(
			"The Confluence HTML could not be parsed.",
			fmt.Errorf("parse Confluence HTML: %w", err),
		)
	}

	writer := htmlMarkdownWriter{}
	writer.writeNode(root, 0)

	return normalizeMarkdown(writer.output.String()), nil
}

// writeNode appends one supported HTML node and recursively processes its children.
func (w *htmlMarkdownWriter) writeNode(node *xhtml.Node, listDepth int) {
	if node.Type == xhtml.TextNode {
		w.writeText(node)
		return
	}
	if node.Type != xhtml.ElementNode && node.Type != xhtml.DocumentNode {
		return
	}

	tag := strings.ToLower(node.Data)
	if skipHTMLTag(tag) {
		return
	}

	listDepth = w.writeOpeningTag(node, tag, listDepth)

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		w.writeNode(child, listDepth)
	}

	w.writeClosingTag(node, tag)
}

// skipHTMLTag reports whether a Confluence HTML subtree should be omitted entirely.
func skipHTMLTag(tag string) bool {
	return tag == "script" || tag == "style" || tag == "nav"
}

// writeOpeningTag emits Markdown that appears before an HTML element's children.
func (w *htmlMarkdownWriter) writeOpeningTag(node *xhtml.Node, tag string, listDepth int) int {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(tag[1] - '0')
		w.output.WriteString("\n\n" + strings.Repeat("#", level) + " ")
	case "p", "div", "section", "article":
		w.output.WriteString("\n\n")
	case "br":
		w.output.WriteByte('\n')
	case "strong", "b":
		w.output.WriteString("**")
	case "em", "i":
		w.output.WriteString("*")
	case "code":
		if !isPreformattedChild(node) {
			w.output.WriteString("`")
		}
	case "pre":
		w.output.WriteString("\n\n```\n")
	case "li":
		w.output.WriteString("\n" + strings.Repeat("  ", listDepth) + "- ")
	case "ul", "ol":
		listDepth++
	case "a":
		w.output.WriteString("[")
	}

	return listDepth
}

// writeClosingTag emits Markdown that appears after an HTML element's children.
func (w *htmlMarkdownWriter) writeClosingTag(node *xhtml.Node, tag string) {
	switch tag {
	case "strong", "b":
		w.output.WriteString("**")
	case "em", "i":
		w.output.WriteString("*")
	case "code":
		if !isPreformattedChild(node) {
			w.output.WriteString("`")
		}
	case "pre":
		w.output.WriteString("\n```\n")
	case "a":
		w.writeAnchorTarget(node)
	case "p", "div", "section", "article", "h1", "h2", "h3", "h4", "h5", "h6":
		w.output.WriteString("\n")
	}
}

// isPreformattedChild reports whether an inline code element belongs to a preformatted block.
func isPreformattedChild(node *xhtml.Node) bool {
	return node.Parent != nil && strings.EqualFold(node.Parent.Data, "pre")
}

// writeText normalizes HTML text-node whitespace while preserving word boundaries.
func (w *htmlMarkdownWriter) writeText(node *xhtml.Node) {
	text := strings.Join(strings.Fields(node.Data), " ")
	if text == "" {
		return
	}

	leadingSpace := strings.TrimLeft(node.Data, " \t\r\n") != node.Data
	trailingSpace := strings.TrimRight(node.Data, " \t\r\n") != node.Data

	if leadingSpace && w.output.Len() > 0 {
		current := w.output.String()
		last := current[len(current)-1]
		if last != ' ' && last != '\n' {
			w.output.WriteByte(' ')
		}
	}

	w.output.WriteString(text)

	if trailingSpace {
		w.output.WriteByte(' ')
	}
}

// writeAnchorTarget closes a Markdown link using the HTML anchor's href attribute.
func (w *htmlMarkdownWriter) writeAnchorTarget(node *xhtml.Node) {
	href := ""

	for _, attr := range node.Attr {
		if attr.Key == "href" {
			href = attr.Val
			break
		}
	}

	w.output.WriteString("](" + href + ")")
}

// normalizeMarkdown collapses repeated blank lines and trims trailing whitespace.
func normalizeMarkdown(source string) string {
	lines := strings.Split(source, "\n")
	cleaned := make([]string, 0, len(lines))
	blank := false

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}

			blank = true
			cleaned = append(cleaned, "")
			continue
		}

		blank = false
		cleaned = append(cleaned, line)
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n")) + "\n"
}
