// Package icons exposes the icon catalogs used by the Kumbuka interface.
package icons

import (
	"cmp"
	"html"
	"html/template"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	lucide "github.com/kaugesaar/lucide-go"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

const (
	lucideSuffix      = "-lucide"
	maxResourceSource = 128
)

//go:generate go run ../../scripts/generate-icons

// Option describes one icon available to icon pickers.
type Option struct {
	// Name is the persisted icon identifier including its source suffix.
	Name string
	// Label is the human-readable name shown in the icon picker.
	Label string
	// Source is the icon pack shown as secondary picker metadata.
	Source string
}

// Resource is one plugin-owned icon catalog declared by an icon-resource module.
type Resource struct {
	// Source records the source associated with resource.
	Source string
	// Data contains the data associated with resource.
	Data []byte
}

// ResourceProvider supplies icon resources from the currently enabled plugins.
// Version must change whenever the active icon-resource set or package versions change.
type ResourceProvider interface {
	IconResourceVersion() string
	IconResources() ([]Resource, error)
}

// Catalog combines Kumbuka's built-in Lucide icons with enabled plugin resources.
type Catalog struct {
	// provider stores the provider value used by catalog.
	provider ResourceProvider

	// mu protects concurrent access to catalog.
	mu sync.RWMutex
	// loaded reports whether loaded applies to catalog.
	loaded bool
	// version stores the version value used by catalog.
	version string
	// options contains the options associated with catalog.
	options []Option
	// icons maps keys to icons values used by catalog.
	icons map[string]pluginpackage.Icon
}

var builtinCatalog = NewCatalog(nil)

// NewCatalog constructs an icon catalog backed by an optional plugin resource provider.
func NewCatalog(provider ResourceProvider) *Catalog {
	return &Catalog{provider: provider}
}

// Builtin returns the process-wide catalog containing only built-in Lucide icons.
func Builtin() *Catalog { return builtinCatalog }

// SVG renders a built-in icon at the requested pixel size.
func SVG(name string, size int) template.HTML { return builtinCatalog.SVG(name, size) }

// Options returns every built-in icon.
func Options() []Option { return builtinCatalog.Options() }

// Search returns at most limit built-in icons matching a name, label, or source.
func Search(query string, limit int) []Option { return builtinCatalog.Search(query, limit) }

// SearchPage returns one page of built-in icons and whether another page is available.
func SearchPage(query string, offset, limit int) ([]Option, bool) {
	return builtinCatalog.SearchPage(query, offset, limit)
}

// IsIcon reports whether a persisted built-in icon identifier is available.
func IsIcon(name string) bool { return builtinCatalog.IsIcon(name) }

// ValidName reports whether a non-empty persisted icon identifier is well formed.
func ValidName(name string) bool { return validIconName(strings.TrimSpace(name)) }

// SVG renders an icon at the requested pixel size.
func (c *Catalog) SVG(name string, size int) template.HTML {
	name = strings.TrimSpace(name)
	if size <= 0 {
		size = 20
	}

	if lucideName, ok := strings.CutSuffix(name, lucideSuffix); ok {
		return lucide.Icon(lucideName, map[string]any{
			"size":  size,
			"class": "lucide-icon",
		})
	}

	c.ensure()
	c.mu.RLock()
	icon, ok := c.icons[name]
	c.mu.RUnlock()
	if !ok {
		return ""
	}

	return resourceSVG(icon, size)
}

// Options returns every icon from the built-in and active plugin catalogs.
func (c *Catalog) Options() []Option {
	c.ensure()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.options)
}

// Search returns at most limit icons matching a name, label, or source.
func (c *Catalog) Search(query string, limit int) []Option {
	options, _ := c.SearchPage(query, 0, limit)
	return options
}

// SearchPage returns one page of icons and whether another page is available.
func (c *Catalog) SearchPage(query string, offset, limit int) (options []Option, hasMore bool) {
	if limit <= 0 {
		return nil, false
	}

	c.ensure()
	c.mu.RLock()
	catalog := c.options
	defer c.mu.RUnlock()

	offset = max(offset, 0)
	query = strings.ToLower(strings.TrimSpace(query))
	options = make([]Option, 0, min(limit+1, len(catalog)))
	matched := 0

	for _, option := range catalog {
		if !matches(option, query) {
			continue
		}
		if matched < offset {
			matched++
			continue
		}

		options = append(options, option)
		if len(options) > limit {
			break
		}
	}

	if len(options) > limit {
		return options[:limit], true
	}

	return options, false
}

// IsIcon reports whether a persisted icon identifier is available.
func (c *Catalog) IsIcon(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}

	c.ensure()
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, found := slices.BinarySearchFunc(c.options, name, func(option Option, candidate string) int {
		return cmp.Compare(option.Name, candidate)
	})
	return found
}

// ensure rebuilds the merged catalog only when the active plugin generation changes.
func (c *Catalog) ensure() {
	version := "builtin"
	if c.provider != nil {
		version = c.provider.IconResourceVersion()
	}

	c.mu.RLock()
	current := c.loaded && c.version == version
	c.mu.RUnlock()
	if current {
		return
	}

	options := slices.Clone(lucideOptions)
	registered := make(map[string]bool, len(options))
	for _, option := range options {
		registered[option.Name] = true
	}
	resources := make(map[string]pluginpackage.Icon)

	if c.provider != nil {
		if contributed, err := c.provider.IconResources(); err == nil {
			for _, resource := range contributed {
				source := strings.TrimSpace(resource.Source)
				if !validResourceText(source, maxResourceSource) {
					continue
				}
				document, err := pluginpackage.ParseIconResource(resource.Data)
				if err != nil {
					continue
				}
				for _, icon := range document.Icons {
					if registered[icon.Name] {
						continue
					}
					registered[icon.Name] = true
					resources[icon.Name] = icon
					options = append(options, Option{Name: icon.Name, Label: icon.Label, Source: source})
				}
			}
		}
	}

	slices.SortFunc(options, func(left, right Option) int {
		return cmp.Compare(left.Name, right.Name)
	})

	c.mu.Lock()
	c.loaded = true
	c.version = version
	c.options = options
	c.icons = resources
	c.mu.Unlock()
}

// validResourceText reports whether a picker source is safe and bounded.
func validResourceText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

// validIconName reports whether an icon name uses the supported identifier syntax.
func validIconName(name string) bool {
	if len(name) == 0 || len(name) > 128 || !isIdentifierStart(name[0]) {
		return false
	}
	for index := 1; index < len(name); index++ {
		character := name[index]
		if !isIdentifierStart(character) && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

// isIdentifierStart reports whether a rune may start an icon identifier.
func isIdentifierStart(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
}

// resourceSVG renders only host-authored SVG structure around escaped path data.
func resourceSVG(icon pluginpackage.Icon, size int) template.HTML {
	var output strings.Builder
	output.WriteString(`<svg class="plugin-icon" width="`)
	output.WriteString(strconv.Itoa(size))
	output.WriteString(`" height="`)
	output.WriteString(strconv.Itoa(size))
	output.WriteString(`" fill="currentColor" aria-hidden="true" focusable="false" viewBox="`)
	output.WriteString(html.EscapeString(icon.ViewBox))
	output.WriteString(`" xmlns="http://www.w3.org/2000/svg">`)
	for _, path := range icon.Paths {
		output.WriteString(`<path d="`)
		output.WriteString(html.EscapeString(path))
		output.WriteString(`"></path>`)
	}
	output.WriteString(`</svg>`)
	return template.HTML(output.String())
}

// matches reports whether an icon name, label, or source contains the normalized query.
func matches(option Option, query string) bool {
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(option.Name), query) ||
		strings.Contains(strings.ToLower(option.Label), query) ||
		strings.Contains(strings.ToLower(option.Source), query)
}
