package markdown

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

// newSanitizer is the trusted final boundary for both authored and generated HTML. Plugins cannot extend this policy or return already-trusted markup.
func newSanitizer() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()

	policy.AllowElements("div", "button", "details", "summary")
	policy.AllowAttrs("class").
		OnElements(
			"aside",
			"pre",
			"code",
			"span",
			"div",
			"button",
			"details",
			"summary",
			"h1",
			"h2",
			"h3",
			"h4",
			"h5",
			"h6",
			"table",
			"thead",
			"tbody",
			"tr",
			"th",
			"td",
		)
	policy.AllowAttrs("role").OnElements("div", "button")
	policy.AllowAttrs("role", "aria-checked", "aria-disabled").OnElements("span")
	policy.AllowAttrs("type", "aria-selected").OnElements("button")
	policy.AllowAttrs("open").OnElements("details")
	policy.AllowAttrs("data-kumbuka-plugin", "data-kumbuka-module").Matching(regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)).OnElements("div", "span")
	policy.AllowAttrs("data-kumbuka-input").Matching(regexp.MustCompile(`^html$`)).OnElements("div", "span")
	policy.AllowAttrs("data-kumbuka-fallback").OnElements("div", "span")
	policy.AllowAttrs("data-kumbuka-mention").OnElements("span")
	policy.AllowAttrs("data-kumbuka-code-block").OnElements("div")
	policy.AllowAttrs("data-plugin-annotation").Matching(regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,191}$`)).OnElements("span")
	policy.AllowAttrs("id").OnElements("h1", "h2", "h3", "h4", "h5", "h6")

	// UGCPolicy's Paragraph filter rejects ordinary image text such as '&' and
	// '{width=50%}'. Keep alt/title as text; the sanitizer still escapes their
	// values and filters URLs, event handlers and styles separately.
	policy.AllowAttrs("alt", "title").OnElements("img")
	policy.AllowStyles("width").
		MatchingHandler(validImageWidthStyle).
		OnElements("img")

	// Allow the navigation elements emitted by trusted render contributions.
	policy.AllowElements("nav")
	policy.AllowAttrs("class").OnElements("nav", "ul", "li", "a", "p")
	policy.AllowAttrs("aria-label").OnElements("nav")
	policy.AllowAttrs("aria-hidden").Matching(regexp.MustCompile(`^(true|false)$`)).OnElements("span", "svg")

	// Preserve static navigation icons using geometry only. No SVG links, use,
	// animation, scripts, foreignObject, styles, or URL-valued paints are allowed.
	shapes := []string{"svg", "g", "path", "circle", "ellipse", "rect", "line", "polyline", "polygon"}
	policy.AllowElements(shapes...)
	policy.AllowAttrs("class").OnElements("svg")
	policy.AllowAttrs("xmlns").Matching(regexp.MustCompile(`^http://www\.w3\.org/2000/svg$`)).OnElements("svg")
	policy.AllowAttrs("viewbox").Matching(regexp.MustCompile(`^[0-9eE+., \-]+$`)).OnElements("svg")
	policy.AllowAttrs("width", "height", "x", "y", "x1", "y1", "x2", "y2", "cx", "cy", "r", "rx", "ry", "stroke-width").Matching(regexp.MustCompile(`^[0-9.]+$`)).OnElements(shapes...)
	policy.AllowAttrs("d").Matching(regexp.MustCompile(`^[MmZzLlHhVvCcSsQqTtAa0-9eE+., \-]+$`)).OnElements("path")
	policy.AllowAttrs("points").Matching(regexp.MustCompile(`^[0-9eE+., \-]+$`)).OnElements("polyline", "polygon")
	policy.AllowAttrs("fill", "stroke").Matching(regexp.MustCompile(`^(none|currentColor)$`)).OnElements(shapes...)
	policy.AllowAttrs("stroke-linecap").Matching(regexp.MustCompile(`^(butt|round|square)$`)).OnElements(shapes...)
	policy.AllowAttrs("stroke-linejoin").Matching(regexp.MustCompile(`^(miter|round|bevel)$`)).OnElements(shapes...)
	policy.AllowAttrs("focusable").Matching(regexp.MustCompile(`^false$`)).OnElements("svg")
	return policy
}
