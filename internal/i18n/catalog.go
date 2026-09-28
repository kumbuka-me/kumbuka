// Package i18n provides request-localized interface messages for Kumbuka.
package i18n

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

const (
	// DefaultCode is the interface language used when no supported preference matches.
	DefaultCode = "en"
	localeGlob  = "locales/*.toml"
)

// Option describes one interface language selectable by a user.
type Option struct {
	// Code is the persisted BCP 47 language tag.
	Code string
	// Label is the native user-facing language name.
	Label string
}

// Catalog contains immutable interface translations loaded at application startup.
type Catalog struct {
	messages map[string]map[string]string
	labels   map[string]string
	codes    []string
	tags     []language.Tag
	matcher  language.Matcher
}

// Localizer resolves translated messages for one request.
type Localizer struct {
	catalog *Catalog
	// Code is the effective BCP 47 interface language tag.
	Code string
}

type catalogFile struct {
	Language string            `toml:"language"`
	Label    string            `toml:"label"`
	Messages map[string]string `toml:"messages"`
}

// Load reads every embedded locale catalog below locales/ and validates the default catalog.
func Load(source fs.FS) (*Catalog, error) {
	paths, err := fs.Glob(source, localeGlob)
	if err != nil {
		return nil, fmt.Errorf("find locale catalogs: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no locale catalogs found at %s", localeGlob)
	}

	sort.Strings(paths)
	catalog := &Catalog{
		messages: make(map[string]map[string]string, len(paths)),
		labels:   make(map[string]string, len(paths)),
	}
	parsedTags := make(map[string]language.Tag, len(paths))

	for _, path := range paths {
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return nil, fmt.Errorf("read locale catalog %s: %w", path, err)
		}

		var file catalogFile
		if err := toml.Unmarshal(data, &file); err != nil {
			return nil, fmt.Errorf("parse locale catalog %s: %w", path, err)
		}

		tag, err := language.Parse(strings.TrimSpace(file.Language))
		if err != nil {
			return nil, fmt.Errorf("parse locale language in %s: %w", path, err)
		}
		code := tag.String()
		if _, exists := catalog.messages[code]; exists {
			return nil, fmt.Errorf("duplicate locale catalog %q", code)
		}
		if strings.TrimSpace(file.Label) == "" {
			return nil, fmt.Errorf("locale catalog %s has no label", path)
		}
		if len(file.Messages) == 0 {
			return nil, fmt.Errorf("locale catalog %s has no messages", path)
		}

		catalog.messages[code] = file.Messages
		catalog.labels[code] = strings.TrimSpace(file.Label)
		parsedTags[code] = tag
	}

	if _, ok := catalog.messages[DefaultCode]; !ok {
		return nil, fmt.Errorf("default locale catalog %q is missing", DefaultCode)
	}

	// Matchers use the first supported language as their fallback. Keep English
	// first regardless of locale filename ordering so unsupported browser locales
	// never select an arbitrary translation.
	catalog.codes = append(catalog.codes, DefaultCode)
	for code := range catalog.messages {
		if code != DefaultCode {
			catalog.codes = append(catalog.codes, code)
		}
	}
	sort.Strings(catalog.codes[1:])
	for _, code := range catalog.codes {
		catalog.tags = append(catalog.tags, parsedTags[code])
	}
	catalog.matcher = language.NewMatcher(catalog.tags)
	return catalog, nil
}

// Options returns the interface languages available for explicit user selection.
func (c *Catalog) Options() []Option {
	options := make([]Option, 0, len(c.codes))
	for _, code := range c.codes {
		options = append(options, Option{Code: code, Label: c.labels[code]})
	}
	sort.Slice(options, func(i, j int) bool {
		return strings.ToLower(options[i].Label) < strings.ToLower(options[j].Label)
	})
	return options
}

// Has reports whether code names an explicitly available interface language.
func (c *Catalog) Has(code string) bool {
	_, ok := c.messages[strings.TrimSpace(code)]
	return ok
}

// Resolve chooses an explicit user locale first, then Accept-Language, then English.
func (c *Catalog) Resolve(preference, acceptLanguage string) Localizer {
	if preference = strings.TrimSpace(preference); preference != "" {
		// Persisted preferences already use one of the catalog codes. Resolve
		// those directly so the common authenticated path avoids reparsing a
		// BCP 47 tag and running the matcher on every page request.
		if _, ok := c.messages[preference]; ok {
			return Localizer{catalog: c, Code: preference}
		}
		if tag, err := language.Parse(preference); err == nil {
			return c.match(tag)
		}
	}

	if tags, _, err := language.ParseAcceptLanguage(acceptLanguage); err == nil && len(tags) > 0 {
		_, index, _ := c.matcher.Match(tags...)
		return Localizer{catalog: c, Code: c.codes[index]}
	}

	return Localizer{catalog: c, Code: DefaultCode}
}

// match resolves one parsed language tag against the supported catalog.
func (c *Catalog) match(tag language.Tag) Localizer {
	_, index, _ := c.matcher.Match(tag)
	return Localizer{catalog: c, Code: c.codes[index]}
}

// Text returns one localized message, falling back to English and finally the key.
func (l Localizer) Text(key string) string {
	if l.catalog == nil {
		return key
	}
	if messages := l.catalog.messages[l.Code]; messages != nil {
		if value, ok := messages[key]; ok {
			return value
		}
	}
	if value, ok := l.catalog.messages[DefaultCode][key]; ok {
		return value
	}
	return key
}

// Textf formats one localized message using fmt-style placeholders.
func (l Localizer) Textf(key string, values ...any) string {
	return fmt.Sprintf(l.Text(key), values...)
}

// TimeAgo formats a timestamp using the request interface language.
func (l Localizer) TimeAgo(value time.Time) string {
	duration := time.Since(value)
	switch {
	case duration < time.Minute:
		return l.Text("time.just_now")
	case duration < time.Hour:
		return l.Textf("time.minutes_ago", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return l.Textf("time.hours_ago", int(duration.Hours()))
	default:
		return value.Format("2006-01-02")
	}
}

// Label returns a localized label for one bounded host-owned enum value.
func (l Localizer) Label(namespace, value string) string {
	key := namespace + "." + strings.ReplaceAll(value, ".", "_")
	translated := l.Text(key)
	if translated == key {
		return value
	}
	return translated
}

// SurfaceLabel returns the localized host label for a plugin widget surface.
func (l Localizer) SurfaceLabel(surface string) string {
	return l.Label("plugin.surface", surface)
}

// BrowserMessages returns the effective browser.* catalog after applying English fallbacks.
func (l Localizer) BrowserMessages() map[string]string {
	messages := make(map[string]string)
	if l.catalog == nil {
		return messages
	}
	for key, value := range l.catalog.messages[DefaultCode] {
		if strings.HasPrefix(key, "browser.") {
			messages[key] = value
		}
	}
	if l.Code == DefaultCode {
		return messages
	}
	for key, value := range l.catalog.messages[l.Code] {
		if strings.HasPrefix(key, "browser.") {
			messages[key] = value
		}
	}
	return messages
}
