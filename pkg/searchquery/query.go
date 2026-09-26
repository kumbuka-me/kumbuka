// Package searchquery parses Kumbuka's user-facing page search language.
package searchquery

import (
	"strings"
	"unicode"
)

// Query separates free-text terms from supported field filters.
type Query struct {
	// Text preserves free-text websearch syntax after supported filters are removed.
	Text string
	// Filters groups supported filter values by their normalized filter name.
	Filters map[string][]string
}

// Parse classifies search tokens without changing their original value semantics.
func Parse(input string) Query {
	var textTerms []string
	filters := make(map[string][]string)
	for _, token := range tokens(input) {
		key, value, ok := splitFilter(token)
		if ok {
			filters[key] = append(filters[key], value)
			continue
		}
		textTerms = append(textTerms, token)
	}
	return Query{Text: strings.Join(textTerms, " "), Filters: filters}
}

// isFilter reports whether a token prefix is a supported field filter.
func isFilter(key string) bool {
	switch key {
	case "tag", "group", "title", "namespace", "author", "status", "owner", "property":
		return true
	default:
		return false
	}
}

// splitFilter recognizes supported field filters and decodes a quoted filter value.
func splitFilter(token string) (string, string, bool) {
	key, value, found := strings.Cut(token, ":")
	key = strings.ToLower(key)
	if !found || value == "" || !isFilter(key) {
		return "", "", false
	}
	return key, unquoteValue(value), true
}

// unquoteValue removes matching filter-value quotes while preserving escaped characters.
func unquoteValue(value string) string {
	if !matchingQuotes(value) {
		return value
	}

	var decoded strings.Builder
	escaped := false
	for _, character := range value[1 : len(value)-1] {
		if escaped {
			decoded.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		decoded.WriteRune(character)
	}
	if escaped {
		decoded.WriteRune('\\')
	}
	return decoded.String()
}

// matchingQuotes reports whether a filter value is enclosed by the same supported quote character.
func matchingQuotes(value string) bool {
	if len(value) < 2 {
		return false
	}
	quote := value[0]
	return (quote == '\'' || quote == '"') && value[len(value)-1] == quote
}

// tokens splits a search query while preserving quoted segments and their delimiters.
func tokens(query string) []string {
	var result []string
	var current strings.Builder
	var quote rune
	escaped := false

	for _, r := range query {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if quote != 0 {
			current.WriteRune(r)
			if r == '\\' {
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			current.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) {
			result = flushToken(result, &current)
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}
	return flushToken(result, &current)
}

// flushToken appends the current token and resets its builder.
func flushToken(tokens []string, current *strings.Builder) []string {
	if current.Len() == 0 {
		return tokens
	}
	tokens = append(tokens, current.String())
	current.Reset()
	return tokens
}
