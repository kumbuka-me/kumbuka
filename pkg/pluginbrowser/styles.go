package pluginbrowser

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/aymerick/douceur/css"
	"github.com/aymerick/douceur/parser"
	"github.com/kumbuka-me/kumbuka/pkg/ascii"
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

type (
	selectorSanitizer    func(string) (string, bool)
	declarationSanitizer func(string, string) (string, string, bool)
)

// PresentationStylesVersion fingerprints the ordered active stylesheet contributions.
func PresentationStylesVersion(manager *plugin.Manager) string {
	var identity strings.Builder
	if manager != nil {
		appendPresentationIdentity(&identity, manager)
	}

	digest := sha256.Sum256([]byte(identity.String()))
	return hex.EncodeToString(digest[:8])
}

// appendPresentationIdentity appends the ordered stylesheet contribution identity used for versioning.
func appendPresentationIdentity(identity *strings.Builder, manager *plugin.Manager) {
	for _, module := range manager.BrowserModules() {
		appendStyleContribution(identity, "browser", module.PluginID, module.ModuleID, module.Digest, module.CSS)
	}
	for _, module := range manager.CodeHighlighters() {
		appendStyleContribution(identity, "highlighter", module.PluginID, module.ModuleID, module.Digest, module.CSS)
	}
	for _, module := range manager.ContentStyles() {
		appendStyleContribution(identity, "content", module.PluginID, module.ModuleID, module.Digest, module.CSS)
	}
}

// appendStyleContribution appends one non-empty stylesheet contribution to the version identity.
func appendStyleContribution(identity *strings.Builder, kind, pluginID, moduleID, digest, path string) {
	if path == "" {
		return
	}

	for _, value := range []string{kind, pluginID, moduleID, digest, path} {
		identity.WriteString(value)
		identity.WriteByte(0)
	}
}

// PresentationStyles publishes only validated presentation declarations from active package stylesheets.
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
		if !ok {
			continue
		}

		output.WriteString(scopedColors(module.PluginID, string(data)))
	}
}

// appendCodeHighlighterStyles appends validated code-highlighter styles.
func appendCodeHighlighterStyles(output *strings.Builder, manager *plugin.Manager) {
	for _, module := range manager.CodeHighlighters() {
		data, ok := loadPresentationAsset(module.CSS, func() ([]byte, error) {
			return manager.CodeHighlighterAsset(module.PluginID, module.Digest, module.CSS)
		})
		if !ok {
			continue
		}

		output.WriteString(scopedCodeStyles(module.PluginID, string(data)))
	}
}

// appendContentModuleStyles appends validated rendered-content styles.
func appendContentModuleStyles(output *strings.Builder, manager *plugin.Manager) {
	for _, module := range manager.ContentStyles() {
		data, ok := loadPresentationAsset(module.CSS, func() ([]byte, error) {
			return manager.ContentStyleAsset(module.PluginID, module.Digest, module.CSS)
		})
		if !ok {
			continue
		}

		output.WriteString(scopedContentStyles(string(data)))
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
	return sanitizeStylesheet(
		source,
		func(selector string) (string, bool) {
			return scopedPresentationSelector(id, selector)
		},
		safePresentationDeclaration,
	)
}

// scopedCodeStyles returns safe highlighter presentation rules scoped to one plugin wrapper.
func scopedCodeStyles(id, source string) string {
	return sanitizeStylesheet(
		source,
		func(selector string) (string, bool) {
			return scopedPresentationSelector(id, selector)
		},
		safeCodeDeclaration,
	)
}

// scopedContentStyles returns safe presentation rules rooted in rendered page content.
func scopedContentStyles(source string) string {
	return sanitizeStylesheet(source, safeContentStyleSelector, safeContentDeclaration)
}

// sanitizeStylesheet parses source and renders only rules accepted by the supplied sanitizers.
func sanitizeStylesheet(
	source string,
	sanitizeSelector selectorSanitizer,
	sanitizeDeclaration declarationSanitizer,
) string {
	sheet, err := parser.Parse(source)
	if err != nil {
		return ""
	}

	var output strings.Builder
	for _, rule := range sheet.Rules {
		output.WriteString(sanitizeQualifiedRule(rule, sanitizeSelector, sanitizeDeclaration))
	}
	return output.String()
}

// sanitizeQualifiedRule renders one qualified rule after filtering its selectors and declarations.
func sanitizeQualifiedRule(
	rule *css.Rule,
	sanitizeSelector selectorSanitizer,
	sanitizeDeclaration declarationSanitizer,
) string {
	if rule.Kind != css.QualifiedRule {
		return ""
	}

	selectors := sanitizeSelectors(rule.Selectors, sanitizeSelector)
	if len(selectors) == 0 {
		return ""
	}

	declarations := sanitizeDeclarations(rule.Declarations, sanitizeDeclaration)
	if len(declarations) == 0 {
		return ""
	}

	return strings.Join(selectors, ",") +
		"{" +
		strings.Join(declarations, "") +
		"}\n"
}

// sanitizeSelectors returns the selectors accepted and normalized by sanitize.
func sanitizeSelectors(selectors []string, sanitize selectorSanitizer) []string {
	result := make([]string, 0, len(selectors))

	for _, selector := range selectors {
		normalized, ok := sanitize(selector)
		if !ok {
			continue
		}

		result = append(result, normalized)
	}

	return result
}

// sanitizeDeclarations returns the declarations accepted and normalized by sanitize.
func sanitizeDeclarations(
	declarations []*css.Declaration,
	sanitize declarationSanitizer,
) []string {
	result := make([]string, 0, len(declarations))

	for _, declaration := range declarations {
		property, value, ok := sanitize(declaration.Property, declaration.Value)
		if !ok {
			continue
		}

		result = append(result, property+":"+value+";")
	}

	return result
}

// scopedPresentationSelector validates a presentation selector and scopes it to one plugin wrapper.
func scopedPresentationSelector(id, selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	selector = strings.TrimSpace(strings.TrimPrefix(selector, ".prose "))
	if !presentationSelector.MatchString(selector) {
		return "", false
	}

	return `[data-kumbuka-plugin="` + id + `"] ` + selector, true
}

// safeContentStyleSelector validates a content selector while preserving its rendered-content scope.
func safeContentStyleSelector(selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	return selector, safeContentSelector(selector)
}

// safePresentationDeclaration validates the color-only presentation subset exposed to browser modules.
func safePresentationDeclaration(property, value string) (string, string, bool) {
	property = normalizeBackgroundProperty(property)

	if !presentationProperty(property) {
		return "", "", false
	}
	if !safeColor(value) {
		return "", "", false
	}

	return property, value, true
}

// safeCodeDeclaration validates the small presentation subset exposed to highlighters.
func safeCodeDeclaration(property, value string) (string, string, bool) {
	property = normalizeBackgroundProperty(property)

	switch property {
	case "background-color", "color", "border-color":
		return property, value, safeColor(value)
	case "font-style":
		return property, value, oneOf(value, "normal", "italic", "oblique")
	case "font-weight":
		return property, value, oneOf(
			value,
			"normal",
			"bold",
			"100",
			"200",
			"300",
			"400",
			"500",
			"600",
			"700",
			"800",
			"900",
		)
	default:
		return "", "", false
	}
}

// normalizeBackgroundProperty maps the supported background shorthand to background-color.
func normalizeBackgroundProperty(property string) string {
	if property == "background" {
		return "background-color"
	}

	return property
}

// presentationProperty reports whether property is allowed in parent-document presentation CSS.
func presentationProperty(property string) bool {
	return oneOf(property, "background-color", "color", "border-color")
}

// safeColor reports whether value uses only the supported color syntax.
func safeColor(value string) bool {
	if !presentationValue.MatchString(value) {
		return false
	}

	for _, match := range presentationFunction.FindAllStringSubmatch(value, -1) {
		if !safeColorFunction(match[1]) {
			return false
		}
	}

	return true
}

// safeColorFunction reports whether name is an allowed CSS color function.
func safeColorFunction(name string) bool {
	return oneOf(name, "var", "color-mix", "rgb", "rgba", "hsl", "hsla")
}

// safeContentSelector limits parent-document plugin CSS to rendered prose and plugin-owned class hooks.
func safeContentSelector(selector string) bool {
	selector = strings.TrimSpace(selector)

	if oneOf(selector, ".prose", ".prose code", ".prose pre") {
		return true
	}
	if !pluginContentSelector(selector) {
		return false
	}

	parts := strings.Fields(strings.TrimPrefix(selector, ".prose "))
	return safeClassSelectors(parts)
}

// pluginContentSelector reports whether selector has the bounded plugin class-selector shape.
func pluginContentSelector(selector string) bool {
	return len(selector) <= 256 && strings.HasPrefix(selector, ".prose .")
}

// safeClassSelectors reports whether every selector part is a safe class selector.
func safeClassSelectors(selectors []string) bool {
	for _, selector := range selectors {
		if !safeClassSelector(selector) {
			return false
		}
	}

	return true
}

// safeClassSelector reports whether a CSS class selector is safe for parent-document publication.
func safeClassSelector(selector string) bool {
	if !strings.HasPrefix(selector, ".") {
		return false
	}

	for _, className := range strings.Split(selector[1:], ".") {
		if !safeClassName(className) {
			return false
		}
	}

	return true
}

// safeClassName reports whether a class name contains only the supported characters.
func safeClassName(name string) bool {
	if name == "" {
		return false
	}

	for index := 0; index < len(name); index++ {
		if !safeClassCharacter(name[index]) {
			return false
		}
	}

	return true
}

// safeClassCharacter reports whether char may appear in a plugin-safe CSS class segment.
func safeClassCharacter(character byte) bool {
	return ascii.IsAlphanumeric(rune(character)) ||
		character == '_' ||
		character == '-'
}

// safeContentDeclaration validates presentation and local layout properties available to rendered-content plugins.
func safeContentDeclaration(property, value string) (string, string, bool) {
	validate := contentValueValidators[property]
	if validate == nil {
		return "", "", false
	}
	if !validate(value) {
		return "", "", false
	}

	return property, value, true
}

// exactCSSValues returns a validator for a small exact CSS value allowlist.
func exactCSSValues(allowed ...string) func(string) bool {
	return func(value string) bool {
		return oneOf(value, allowed...)
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
	dotSeen := false
	digitSeen := false

	for _, char := range value {
		switch {
		case ascii.IsDigit(char):
			digitSeen = true
		case char == '.' && !dotSeen:
			dotSeen = true
		default:
			return false
		}
	}

	return digitSeen
}

// safeTypographyValue rejects CSS constructs that can load resources or escape a declaration.
func safeTypographyValue(value string) bool {
	if len(value) == 0 || len(value) > 512 {
		return false
	}

	lower := strings.ToLower(value)
	for _, forbidden := range []string{
		"url(",
		"expression(",
		"@",
		"{",
		"}",
		";",
		"<",
		">",
		"\\",
	} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}

	return true
}
