package site

import (
	"bytes"
	"cmp"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	md "github.com/kumbuka-me/kumbuka/internal/markdown"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type pageDiscovery struct {
	sourceDir string
	pages     []sourcePage
	routes    map[string]string
}

// discoverPages discovers Markdown source files and maps them to static routes.
func discoverPages(sourceDir string) ([]sourcePage, error) {
	discovery := pageDiscovery{
		sourceDir: sourceDir,
		routes:    make(map[string]string),
	}

	if err := filepath.WalkDir(sourceDir, discovery.visit); err != nil {
		return nil, err
	}

	slices.SortFunc(discovery.pages, compareSourcePages)
	return discovery.pages, nil
}

// visit collects one Markdown file during source directory traversal.
func (d *pageDiscovery) visit(filename string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if filename == d.sourceDir {
		return nil
	}
	if entry.IsDir() {
		if strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		return nil
	}
	if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
		return nil
	}

	relative, err := filepath.Rel(d.sourceDir, filename)
	if err != nil {
		return err
	}
	relative = filepath.ToSlash(relative)

	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	route := markdownFileRoute(relative)
	if existing, found := d.routes[route]; found {
		return fmt.Errorf("markdown files %s and %s map to the same route %q", existing, relative, route)
	}

	d.routes[route] = relative
	title, hasTitle := markdownTitle(string(data), route)
	d.pages = append(d.pages, sourcePage{
		SourcePath:      relative,
		Route:           route,
		Title:           title,
		Markdown:        string(data),
		HasTitleHeading: hasTitle,
	})

	return nil
}

// compareSourcePages orders discovered pages by source path.
func compareSourcePages(left, right sourcePage) int {
	return cmp.Compare(left.SourcePath, right.SourcePath)
}

// hasHomePage reports whether the discovered pages contain the root route.
func hasHomePage(pages []sourcePage) bool {
	return slices.ContainsFunc(pages, func(page sourcePage) bool {
		return page.Route == ""
	})
}

// indexPages builds source-route and wiki-link lookup indexes.
func indexPages(pages []sourcePage) (map[string]string, map[string]string) {
	routesBySource := make(map[string]string, len(pages))
	wikiTargets := make(map[string]string, len(pages)*2)
	ambiguousWikiTargets := make(map[string]bool)

	for _, page := range pages {
		routesBySource[page.SourcePath] = page.Route
		registerWikiTarget(wikiTargets, ambiguousWikiTargets, md.Slug(page.Route), page.Route)
		registerWikiTarget(wikiTargets, ambiguousWikiTargets, md.Slug(page.Title), page.Route)
	}
	for target := range ambiguousWikiTargets {
		delete(wikiTargets, target)
	}

	return routesBySource, wikiTargets
}

// markdownFileRoute maps a Markdown source filename to its clean static route.
func markdownFileRoute(filename string) string {
	clean := strings.TrimPrefix(path.Clean("/"+filepath.ToSlash(filename)), "/")
	clean = strings.TrimSuffix(clean, path.Ext(clean))

	if path.Base(clean) == "index" {
		clean = path.Dir(clean)
		if clean == "." {
			clean = ""
		}
	}

	return strings.Trim(clean, "/")
}

// markdownTitle extracts the first level-one heading or derives a title from the route.
func markdownTitle(source, route string) (title string, hasTitle bool) {
	fence := ""
	for line := range strings.SplitSeq(strings.TrimPrefix(source, "\ufeff"), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			if fence == "" {
				fence = marker
			} else if marker == fence {
				fence = ""
			}
			continue
		}

		if fence == "" {
			title, ok := strings.CutPrefix(trimmed, "# ")
			if ok && strings.TrimSpace(title) != "" {
				return strings.TrimSpace(title), true
			}
		}
	}

	if route == "" {
		return "Home", false
	}

	segment := path.Base(route)
	segment = strings.NewReplacer("-", " ", "_", " ").Replace(segment)
	words := strings.Fields(segment)
	for index, word := range words {
		runes := []rune(word)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			words[index] = string(runes)
		}
	}

	return strings.Join(words, " "), false
}

// registerWikiTarget records an unambiguous wiki-link target.
func registerWikiTarget(targets map[string]string, ambiguous map[string]bool, target, route string) {
	target = strings.Trim(target, "/")
	if target == "" && route != "" {
		return
	}
	if existing, found := targets[target]; found && existing != route {
		ambiguous[target] = true
		return
	}

	targets[target] = route
}

// validateWikiLinks rejects source pages that reference unresolved wiki targets.
func validateWikiLinks(page sourcePage, targets map[string]string) error {
	for _, target := range md.Links(page.Markdown) {
		if _, found := targets[target]; !found {
			return fmt.Errorf("%s contains unresolved wiki link %q", page.SourcePath, target)
		}
	}

	return nil
}

// expandedPrefixes returns navigation ancestors that should be expanded for a route.
func expandedPrefixes(route string) []string {
	parts := strings.Split(strings.Trim(route, "/"), "/")
	if len(parts) <= 1 {
		return nil
	}

	expanded := make([]string, 0, len(parts)-1)
	for index := 1; index < len(parts); index++ {
		expanded = append(expanded, strings.Join(parts[:index], "/"))
	}

	return expanded
}

// processRenderedHTML removes the duplicate title, rewrites local URLs, and extracts search text.
func processRenderedHTML(
	rendered, sourcePath string,
	removeTitle bool,
	routesBySource map[string]string,
	basePath string,
) (renderedHTML string, searchText string, err error) {
	contextNode := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(rendered), contextNode)
	if err != nil {
		return "", "", err
	}

	if removeTitle {
		nodes = removeFirstHeading(nodes)
	}
	for _, node := range nodes {
		if err := rewriteHTMLURLs(node, sourcePath, routesBySource, basePath); err != nil {
			return "", "", err
		}
	}

	var htmlOutput bytes.Buffer
	for _, node := range nodes {
		if err := xhtml.Render(&htmlOutput, node); err != nil {
			return "", "", err
		}
	}

	return htmlOutput.String(), normalizeSearchText(textFromNodes(nodes)), nil
}

// removeFirstHeading removes the first top-level heading node from rendered fragments.
func removeFirstHeading(nodes []*xhtml.Node) []*xhtml.Node {
	for index, node := range nodes {
		if node.Type == xhtml.ElementNode && node.Data == "h1" {
			return slices.Delete(nodes, index, index+1)
		}
	}

	return nodes
}

// isRewritableURLAttribute reports whether a static-page attribute contains a navigable local URL.
func isRewritableURLAttribute(element, attribute string) bool {
	switch element {
	case "a":
		return attribute == "href"
	case "img":
		return attribute == "src"
	default:
		return false
	}
}

// rewriteHTMLURLs recursively rewrites navigable local URLs in rendered HTML.
func rewriteHTMLURLs(node *xhtml.Node, sourcePath string, routesBySource map[string]string, basePath string) error {
	if node.Type == xhtml.ElementNode {
		for index := range node.Attr {
			attribute := &node.Attr[index]
			if !isRewritableURLAttribute(node.Data, attribute.Key) {
				continue
			}

			rewritten, err := rewriteLocalURL(attribute.Val, sourcePath, routesBySource, basePath)
			if err != nil {
				return err
			}
			attribute.Val = rewritten
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if err := rewriteHTMLURLs(child, sourcePath, routesBySource, basePath); err != nil {
			return err
		}
	}

	return nil
}

// isRewritableLocalURL reports whether a parsed URL refers to a non-empty path inside the generated site.
func isRewritableLocalURL(value string, parsed *url.URL) bool {
	if parsed.IsAbs() || parsed.Host != "" {
		return false
	}
	if strings.HasPrefix(value, "//") {
		return false
	}

	return parsed.Path != ""
}

// rewriteLocalURL rewrites one local Markdown or asset URL for the generated site.
func rewriteLocalURL(value, sourcePath string, routesBySource map[string]string, basePath string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "#") {
		return value, nil
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if !isRewritableLocalURL(value, parsed) {
		return value, nil
	}

	basePath = ensureBasePath(basePath)
	if basePath != "/" && strings.HasPrefix(parsed.Path, basePath) {
		return value, nil
	}

	trailingSlash := strings.HasSuffix(parsed.Path, "/")
	resolved := resolveLocalPath(parsed.Path, sourcePath)
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", fmt.Errorf("link %q escapes the documentation source", value)
	}

	if strings.EqualFold(path.Ext(resolved), ".md") {
		route, found := routesBySource[resolved]
		if !found {
			return "", fmt.Errorf("markdown link %q points to missing file %s", value, resolved)
		}
		parsed.Path = pageURL(basePath, route)
	} else {
		parsed.Path = basePath + strings.TrimPrefix(resolved, "/")
		if trailingSlash && !strings.HasSuffix(parsed.Path, "/") {
			parsed.Path += "/"
		}
	}

	return parsed.String(), nil
}

// resolveLocalPath resolves one URL path relative to its Markdown source file.
func resolveLocalPath(value, sourcePath string) string {
	if strings.HasPrefix(value, "/") {
		return strings.TrimPrefix(path.Clean(value), "/")
	}

	resolved := path.Clean(path.Join(path.Dir(sourcePath), value))
	if resolved == "." {
		return ""
	}

	return resolved
}

// textFromNodes extracts searchable text from rendered HTML nodes.
func textFromNodes(nodes []*xhtml.Node) string {
	var output strings.Builder
	for _, node := range nodes {
		appendNodeText(&output, node)
	}

	return output.String()
}

// appendNodeText recursively appends searchable text while skipping scripts and styles.
func appendNodeText(output *strings.Builder, node *xhtml.Node) {
	if node.Type == xhtml.TextNode {
		output.WriteString(node.Data)
		output.WriteByte(' ')
	}
	if node.Type == xhtml.ElementNode && (node.Data == "script" || node.Data == "style") {
		return
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendNodeText(output, child)
	}
}

// normalizeSearchText collapses rendered text into a single whitespace-normalized string.
func normalizeSearchText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
