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
	for _, module := range manager.BrowserModules() {
		if module.CSS == "" {
			continue
		}
		data, err := manager.BrowserAsset(module.PluginID, module.Digest, module.CSS)
		if err != nil || len(data) > 256<<10 {
			continue
		}
		output.WriteString(scopedColors(module.PluginID, string(data)))
	}
	for _, module := range manager.CodeHighlighters() {
		if module.CSS == "" {
			continue
		}
		data, err := manager.CodeHighlighterAsset(module.PluginID, module.Digest, module.CSS)
		if err != nil || len(data) > 256<<10 {
			continue
		}
		output.WriteString(scopedCodeStyles(module.PluginID, string(data)))
	}
	for _, module := range manager.ContentStyles() {
		if module.CSS == "" {
			continue
		}
		data, err := manager.ContentStyleAsset(module.PluginID, module.Digest, module.CSS)
		if err != nil || len(data) > 256<<10 {
			continue
		}
		output.WriteString(scopedContentStyles(string(data)))
	}
	return output.String()
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
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			segmentStart = false
			continue
		}
		return false
	}
	return !segmentStart
}

// safeContentDeclaration validates presentation and local layout properties available to rendered-content plugins.
func safeContentDeclaration(property, value string) (string, string, bool) {
	switch property {
	case "font-family", "font-feature-settings", "font-variant-ligatures":
		return property, value, safeTypographyValue(value)
	case "color", "background-color", "border-color", "border-top-color", "border-right-color", "border-bottom-color", "border-left-color":
		return property, value, safeColor(value)
	case "list-style":
		return property, value, value == "none"
	case "margin-top", "margin-right", "margin-bottom", "margin-left",
		"padding-top", "padding-right", "padding-bottom", "padding-left",
		"border-width", "border-top-width", "border-right-width", "border-bottom-width", "border-left-width", "border-radius",
		"gap", "font-size", "min-width", "max-width", "width":
		return property, value, safeContentLength(value)
	case "border-style", "border-top-style", "border-right-style", "border-bottom-style", "border-left-style":
		return property, value, value == "solid" || value == "none"
	case "display":
		return property, value, oneOf(value, "block", "inline-block", "flex", "inline-flex")
	case "align-items":
		return property, value, oneOf(value, "stretch", "center", "flex-start", "flex-end")
	case "justify-content":
		return property, value, oneOf(value, "flex-start", "flex-end", "center", "space-between")
	case "flex":
		return property, value, oneOf(value, "0 0 auto", "1 1 auto")
	case "flex-wrap":
		return property, value, oneOf(value, "nowrap", "wrap")
	case "overflow", "overflow-x":
		return property, value, oneOf(value, "visible", "hidden", "auto")
	case "overflow-wrap":
		return property, value, oneOf(value, "normal", "break-word", "anywhere")
	case "white-space":
		return property, value, oneOf(value, "normal", "nowrap", "pre", "pre-wrap")
	case "text-align":
		return property, value, oneOf(value, "left", "right", "center")
	case "font-weight":
		return property, value, oneOf(value, "400", "500", "600", "700")
	case "user-select":
		return property, value, oneOf(value, "none", "text")
	default:
		return "", "", false
	}
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
	unit := ""
	for _, candidate := range []string{"rem", "em", "px", "%"} {
		if strings.HasSuffix(value, candidate) {
			unit = candidate
			break
		}
	}
	if unit == "" {
		return false
	}
	number := strings.TrimSuffix(value, unit)
	if number == "" {
		return false
	}
	dot := false
	digit := false
	for _, char := range number {
		switch {
		case char >= '0' && char <= '9':
			digit = true
		case char == '.' && !dot:
			dot = true
		default:
			return false
		}
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
