package pluginbrowser

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/aymerick/douceur/css"
	"github.com/aymerick/douceur/parser"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

var (
	presentationSelector = regexp.MustCompile(`^[a-zA-Z0-9 .>:_-]{1,256}$`)
	presentationValue    = regexp.MustCompile(`^[a-zA-Z0-9 #(),.%_-]{1,512}$`)
	presentationFunction = regexp.MustCompile(`([a-zA-Z_-]+)\s*\(`)
)

var contentValueValidators = map[string]func(string) bool{
	"font-family":            safeTypographyValue,
	"font-feature-settings":  safeTypographyValue,
	"font-variant-ligatures": safeTypographyValue,
	"color":                  safeColor,
	"background-color":       safeColor,
	"border-color":           safeColor,
	"border-top-color":       safeColor,
	"border-right-color":     safeColor,
	"border-bottom-color":    safeColor,
	"border-left-color":      safeColor,
	"list-style":             exactCSSValues("none"),
	"margin-top":             safeContentLength,
	"margin-right":           safeContentLength,
	"margin-bottom":          safeContentLength,
	"margin-left":            safeContentLength,
	"padding-top":            safeContentLength,
	"padding-right":          safeContentLength,
	"padding-bottom":         safeContentLength,
	"padding-left":           safeContentLength,
	"border-width":           safeContentLength,
	"border-top-width":       safeContentLength,
	"border-right-width":     safeContentLength,
	"border-bottom-width":    safeContentLength,
	"border-left-width":      safeContentLength,
	"border-radius":          safeContentLength,
	"gap":                    safeContentLength,
	"font-size":              safeContentLength,
	"min-width":              safeContentLength,
	"max-width":              safeContentLength,
	"width":                  safeContentLength,
	"border-style":           exactCSSValues("solid", "none"),
	"border-top-style":       exactCSSValues("solid", "none"),
	"border-right-style":     exactCSSValues("solid", "none"),
	"border-bottom-style":    exactCSSValues("solid", "none"),
	"border-left-style":      exactCSSValues("solid", "none"),
	"display":                exactCSSValues("block", "inline-block", "flex", "inline-flex"),
	"align-items":            exactCSSValues("stretch", "center", "flex-start", "flex-end"),
	"justify-content":        exactCSSValues("flex-start", "flex-end", "center", "space-between"),
	"flex":                   exactCSSValues("0 0 auto", "1 1 auto"),
	"flex-wrap":              exactCSSValues("nowrap", "wrap"),
	"overflow":               exactCSSValues("visible", "hidden", "auto"),
	"overflow-x":             exactCSSValues("visible", "hidden", "auto"),
	"overflow-wrap":          exactCSSValues("normal", "break-word", "anywhere"),
	"white-space":            exactCSSValues("normal", "nowrap", "pre", "pre-wrap"),
	"text-align":             exactCSSValues("left", "right", "center"),
	"font-weight":            exactCSSValues("400", "500", "600", "700"),
	"user-select":            exactCSSValues("none", "text"),
}

// PresentationStylesVersion fingerprints the ordered active stylesheet contributions. Package digests already cover the referenced CSS bytes, so this stays cheap enough to compute while rendering each page without reparsing stylesheet contents.
func PresentationStylesVersion(manager *plugin.Manager) string {
	var identity strings.Builder
	appendContribution := func(kind, pluginID, moduleID, digest, css string) {
		if css == "" {
			return
		}
		for _, value := range []string{kind, pluginID, moduleID, digest, css} {
			identity.WriteString(value)
			identity.WriteByte(0)
		}
	}

	if manager != nil {
		for _, module := range manager.BrowserModules() {
			appendContribution("browser", module.PluginID, module.ModuleID, module.Digest, module.CSS)
		}
		for _, module := range manager.CodeHighlighters() {
			appendContribution("highlighter", module.PluginID, module.ModuleID, module.Digest, module.CSS)
		}
		for _, module := range manager.ContentStyles() {
			appendContribution("content", module.PluginID, module.ModuleID, module.Digest, module.CSS)
		}
	}

	digest := sha256.Sum256([]byte(identity.String()))
	return hex.EncodeToString(digest[:8])
}

// PresentationStyles publishes only scoped presentation declarations from active package stylesheets. Arbitrary CSS stays in the sandbox: positioning, URLs, generated content, imports, escapes and selector functions are not admitted.
func PresentationStyles(manager *plugin.Manager) string {
	if manager == nil {
		return ""
	}
	var output strings.Builder
	appendBrowserModuleStyles(&output, manager)
	appendCodeHighlighterStyles(&output, manager)
	appendContentModuleStyles(&output, manager)
	return output.String()
}

// appendBrowserModuleStyles appends validated browser-module color styles.
func appendBrowserModuleStyles(output *strings.Builder, manager *plugin.Manager) {
	for _, module := range manager.BrowserModules() {
		data, ok := loadPresentationAsset(module.CSS, func() ([]byte, error) {
			return manager.BrowserAsset(module.PluginID, module.Digest, module.CSS)
		})
		if ok {
			output.WriteString(scopedColors(module.PluginID, string(data)))
		}
	}
}

// appendCodeHighlighterStyles appends validated code-highlighter styles.
func appendCodeHighlighterStyles(output *strings.Builder, manager *plugin.Manager) {
	for _, module := range manager.CodeHighlighters() {
		data, ok := loadPresentationAsset(module.CSS, func() ([]byte, error) {
			return manager.CodeHighlighterAsset(module.PluginID, module.Digest, module.CSS)
		})
		if ok {
			output.WriteString(scopedCodeStyles(module.PluginID, string(data)))
		}
	}
}

// appendContentModuleStyles appends validated rendered-content styles.
func appendContentModuleStyles(output *strings.Builder, manager *plugin.Manager) {
	for _, module := range manager.ContentStyles() {
		data, ok := loadPresentationAsset(module.CSS, func() ([]byte, error) {
			return manager.ContentStyleAsset(module.PluginID, module.Digest, module.CSS)
		})
		if ok {
			output.WriteString(scopedContentStyles(string(data)))
		}
	}
}

// loadPresentationAsset loads one bounded stylesheet contribution.
func loadPresentationAsset(path string, load func() ([]byte, error)) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	data, err := load()
	return data, err == nil && len(data) <= 256<<10
}

// scopedColors returns only safe, plugin-scoped color declarations from source.
func scopedColors(id, source string) string {
	sheet, err := parser.Parse(source)
	if err != nil {
		return ""
	}
	var output strings.Builder
	for _, rule := range sheet.Rules {
		if rule.Kind != css.QualifiedRule {
			continue
		}
		var selectors []string
		for _, selector := range rule.Selectors {
			selector = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(selector), ".prose "))
			if !presentationSelector.MatchString(selector) {
				continue
			}
			selectors = append(selectors, `[data-kumbuka-plugin="`+id+`"] `+selector)
		}
		if len(selectors) == 0 {
			continue
		}
		var declarations []string
		for _, declaration := range rule.Declarations {
			property := declaration.Property
			if property == "background" {
				property = "background-color"
			}
			if !presentationProperty(property) {
				continue
			}
			if !safeColor(declaration.Value) {
				continue
			}
			declarations = append(declarations, property+":"+declaration.Value+";")
		}
		if len(declarations) > 0 {
			output.WriteString(strings.Join(selectors, ",") + "{" + strings.Join(declarations, "") + "}\n")
		}
	}
	return output.String()
}

// scopedCodeStyles returns safe highlighter presentation rules scoped to one plugin wrapper.
func scopedCodeStyles(id, source string) string {
	sheet, err := parser.Parse(source)
	if err != nil {
		return ""
	}

	var output strings.Builder
	for _, rule := range sheet.Rules {
		if rule.Kind != css.QualifiedRule {
			continue
		}

		selectors := make([]string, 0, len(rule.Selectors))
		for _, selector := range rule.Selectors {
			selector = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(selector), ".prose "))
			if !presentationSelector.MatchString(selector) {
				continue
			}
			selectors = append(selectors, `[data-kumbuka-plugin="`+id+`"] `+selector)
		}
		if len(selectors) == 0 {
			continue
		}

		declarations := make([]string, 0, len(rule.Declarations))
		for _, declaration := range rule.Declarations {
			property, value, ok := safeCodeDeclaration(declaration.Property, declaration.Value)
			if ok {
				declarations = append(declarations, property+":"+value+";")
			}
		}
		if len(declarations) != 0 {
			output.WriteString(strings.Join(selectors, ",") + "{" + strings.Join(declarations, "") + "}\n")
		}
	}

	return output.String()
}

// safeCodeDeclaration validates the small presentation subset exposed to highlighters.
func safeCodeDeclaration(property, value string) (string, string, bool) {
	switch property {
	case "background":
		property = "background-color"
		fallthrough
	case "background-color", "color", "border-color":
		return property, value, safeColor(value)
	case "font-style":
		return property, value, value == "normal" || value == "italic" || value == "oblique"
	case "font-weight":
		switch value {
		case "normal", "bold", "100", "200", "300", "400", "500", "600", "700", "800", "900":
			return property, value, true
		}
	}

	return "", "", false
}

// presentationProperty reports whether property is allowed in parent-document presentation CSS.
func presentationProperty(property string) bool {
	switch property {
	case "background-color", "color", "border-color":
		return true
	default:
		return false
	}
}

// safeColor reports whether value uses only the supported color syntax.
func safeColor(value string) bool {
	if !presentationValue.MatchString(value) {
		return false
	}
	for _, match := range presentationFunction.FindAllStringSubmatch(value, -1) {
		switch match[1] {
		case "var", "color-mix", "rgb", "rgba", "hsl", "hsla":
		default:
			return false
		}
	}
	return true
}

// scopedContentStyles returns safe presentation rules rooted in rendered page content.
func scopedContentStyles(source string) string {
	sheet, err := parser.Parse(source)
	if err != nil {
		return ""
	}

	var output strings.Builder
	for _, rule := range sheet.Rules {
		if rule.Kind != css.QualifiedRule {
			continue
		}

		selectors := make([]string, 0, len(rule.Selectors))
		for _, selector := range rule.Selectors {
			selector = strings.TrimSpace(selector)
			if safeContentSelector(selector) {
				selectors = append(selectors, selector)
			}
		}
		if len(selectors) == 0 {
			continue
		}

		declarations := make([]string, 0, len(rule.Declarations))
		for _, declaration := range rule.Declarations {
			property, value, ok := safeContentDeclaration(declaration.Property, declaration.Value)
			if !ok {
				continue
			}
			declarations = append(declarations, property+":"+value+";")
		}
		if len(declarations) != 0 {
			output.WriteString(strings.Join(selectors, ",") + "{" + strings.Join(declarations, "") + "}\n")
		}
	}

	return output.String()
}

// safeContentSelector limits parent-document plugin CSS to rendered prose and plugin-owned class hooks. Complex selectors, pseudo classes, IDs and attributes stay unavailable so a content plugin cannot reach application chrome.
func safeContentSelector(selector string) bool {
	selector = strings.TrimSpace(selector)
	switch selector {
	case ".prose", ".prose code", ".prose pre":
		return true
	}
	if !strings.HasPrefix(selector, ".prose .") || len(selector) > 256 {
		return false
	}
	for _, part := range strings.Fields(strings.TrimPrefix(selector, ".prose ")) {
		if !safeClassSelector(part) {
			return false
		}
	}
	return true
}

// safeClassSelector reports whether a CSS class selector is safe for parent-document publication.
func safeClassSelector(selector string) bool {
	if len(selector) < 2 || selector[0] != '.' {
		return false
	}
	segmentStart := true
	for index := 1; index < len(selector); index++ {
		char := selector[index]
		if char == '.' {
			if segmentStart {
				return false
			}
			segmentStart = true
			continue
		}
		if safeClassCharacter(char) {
			segmentStart = false
			continue
		}
		return false
	}
	return !segmentStart
}

// safeClassCharacter reports whether char may appear in a plugin-safe CSS class segment.
func safeClassCharacter(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' ||
		char >= '0' && char <= '9' || char == '_' || char == '-'
}

// safeContentDeclaration validates presentation and local layout properties available to rendered-content plugins.
func safeContentDeclaration(property, value string) (string, string, bool) {
	validate := contentValueValidators[property]
	if validate == nil || !validate(value) {
		return "", "", false
	}
	return property, value, true
}

// exactCSSValues returns a validator for a small exact CSS value allowlist.
func exactCSSValues(allowed ...string) func(string) bool {
	return func(value string) bool { return oneOf(value, allowed...) }
}

// oneOf reports whether value is one exact member of the allowlist.
func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

// safeContentLength reports whether a CSS declaration value stays within the configured bound.
func safeContentLength(value string) bool {
	if value == "0" {
		return true
	}
	if len(value) < 3 || len(value) > 32 {
		return false
	}

	number, ok := splitContentLength(value)
	return ok && validContentLengthNumber(number)
}

// splitContentLength removes one allowed CSS length unit.
func splitContentLength(value string) (string, bool) {
	for _, unit := range []string{"rem", "em", "px", "%"} {
		if strings.HasSuffix(value, unit) {
			number := strings.TrimSuffix(value, unit)
			return number, number != ""
		}
	}
	return "", false
}

// validContentLengthNumber validates a non-negative decimal number without exponent or sign syntax.
func validContentLengthNumber(value string) bool {
	dot := false
	digit := false
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digit = true
			continue
		}
		if char != '.' || dot {
			return false
		}
		dot = true
	}
	return digit
}

// safeTypographyValue rejects CSS constructs that can load resources or escape a declaration.
func safeTypographyValue(value string) bool {
	if len(value) == 0 || len(value) > 512 {
		return false
	}
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"url(", "expression(", "@", "{", "}", ";", "<", ">", "\\"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return true
}
