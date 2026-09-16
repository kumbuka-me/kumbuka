package navigation

import (
	"cmp"
	"slices"
	"strings"
)

// Page contains the page metadata required to build navigation.
type Page struct {
	// Slug is the complete page path.
	Slug string
	// Title is the page label displayed to the user.
	Title string
	// Icon is the optional icon configured for the page.
	Icon string
}

// Options controls the presentation state applied while building the navigation tree.
type Options struct {
	// ActiveSlug identifies the page currently being viewed or edited.
	ActiveSlug string
	// Expanded contains folder slugs explicitly expanded by the user.
	Expanded []string
	// ShowPageCounts enables descendant page counts in rendered navigation nodes.
	ShowPageCounts bool
	// Icons contains explicitly configured icons keyed by complete navigation path.
	Icons map[string]string
}

// Node is one page or folder in the slug-derived navigation tree.
type Node struct {
	// Title is the display label for the page or folder.
	Title string
	// Slug is the accumulated navigation path for the node.
	Slug string
	// Icon is the explicitly configured icon for this navigation path.
	Icon string
	// Root reports whether the node is a top-level navigation entry.
	Root bool
	// Page reports whether the node maps to a real page.
	Page bool
	// Active reports whether this node is the current page.
	Active bool
	// ContainsActive reports whether this node or one of its descendants is active.
	ContainsActive bool
	// Open reports whether a folder should initially render expanded.
	Open bool
	// PageCount is the number of real pages represented by this node and its descendants.
	PageCount int
	// ShowPageCount reports whether the page count should be rendered.
	ShowPageCount bool
	// Children contains nested navigation nodes.
	Children []Node
}

// branch is the mutable internal representation used while building navigation.
type branch struct {
	// title is the display label for the branch.
	title string
	// slug is the accumulated navigation path for the branch.
	slug string
	// icon is the explicitly configured icon for this branch.
	icon string
	// page reports whether the branch maps to a real page.
	page bool
	// children indexes nested branches by slug segment.
	children map[string]*branch
}

// Build derives navigation from page slugs, persisted metadata, and user presentation state.
func Build(pages []Page, options Options) []Node {
	root := &branch{children: map[string]*branch{}}

	for _, page := range pages {
		parts := strings.Split(strings.Trim(page.Slug, "/"), "/")
		current := root

		for index, part := range parts {
			if part == "" {
				continue
			}

			child := current.children[part]

			if child == nil {
				slug := part

				if current.slug != "" {
					slug = current.slug + "/" + part
				}

				child = &branch{title: part, slug: slug, children: map[string]*branch{}}
				current.children[part] = child
			}
			if icon := options.Icons[child.slug]; icon != "" {
				child.icon = icon
			}
			if index == len(parts)-1 {
				child.page = true
				child.title = page.Title

				if page.Icon != "" {
					child.icon = page.Icon
				}
			}

			current = child
		}
	}

	expanded := make(map[string]bool, len(options.Expanded))

	for _, slug := range options.Expanded {
		slug = strings.Trim(strings.TrimSpace(slug), "/")
		if slug != "" {
			expanded[slug] = true
		}
	}

	activeSlug := strings.Trim(strings.TrimSpace(options.ActiveSlug), "/")

	return nodes(root, 0, activeSlug, expanded, options.ShowPageCounts)
}

// Children returns the direct children of slug, including their nested descendants.
func Children(tree []Node, slug string) []Node {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	children, _ := children(tree, slug)

	return children
}

// children recursively locates a node while distinguishing a leaf from no match.
func children(tree []Node, slug string) (descendants []Node, found bool) {
	for _, node := range tree {
		if node.Slug == slug {
			return node.Children, true
		}
		if descendants, found := children(node.Children, slug); found {
			return descendants, true
		}
	}
	return nil, false
}

// nodes recursively converts internal branches into sorted navigation nodes.
func nodes(parent *branch, depth int, activeSlug string, expanded map[string]bool, showPageCounts bool) []Node {
	result := make([]Node, 0, len(parent.children))

	for _, child := range parent.children {
		children := nodes(child, depth+1, activeSlug, expanded, showPageCounts)
		active := child.page && child.slug == activeSlug
		containsActive := active
		pageCount := 0

		if child.page {
			pageCount++
		}
		for _, nested := range children {
			pageCount += nested.PageCount
			if nested.ContainsActive {
				containsActive = true
			}
		}

		result = append(result, Node{
			Title:          child.title,
			Slug:           child.slug,
			Icon:           child.icon,
			Root:           depth == 0,
			Page:           child.page,
			Active:         active,
			ContainsActive: containsActive,
			Open:           expanded[child.slug],
			PageCount:      pageCount,
			ShowPageCount:  showPageCounts,
			Children:       children,
		})
	}

	slices.SortFunc(result, compareNodes)
	return result
}

// compareNodes orders folders before leaves, then compares titles case-insensitively.
func compareNodes(left, right Node) int {
	leftFolder, rightFolder := len(left.Children) > 0, len(right.Children) > 0
	switch {
	case leftFolder && !rightFolder:
		return -1
	case !leftFolder && rightFolder:
		return 1
	default:
		return cmp.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
	}
}
