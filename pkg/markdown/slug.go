package markdown

import (
	"strings"
	"unicode"

	"github.com/kumbuka-me/kumbuka/internal/ascii"
)

// Slug converts human-readable page text into a canonical page slug.
func Slug(value string) string {
	value = strings.TrimSpace(value)
	var output strings.Builder
	separator := false

	for _, r := range value {
		r = unicode.ToLower(r)
		switch {
		case isSlugRune(r):
			if separator && output.Len() > 0 {
				output.WriteByte('-')
			}

			separator = false

			output.WriteRune(r)
		default:
			separator = true
		}
	}

	return strings.Trim(output.String(), "-")
}

// isSlugRune reports whether character can be preserved in a canonical page slug.
func isSlugRune(character rune) bool {
	return ascii.IsAlphanumeric(character) || character == '/' || character == '_' || character == '-'
}
