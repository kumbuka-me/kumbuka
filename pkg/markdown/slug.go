package markdown

import (
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/ascii"
)

// Slug converts human-readable page text into a canonical page slug.
func Slug(value string) string {
	value = strings.TrimSpace(value)
	var output strings.Builder
	output.Grow(len(value))
	separator := false

	for index := 0; index < len(value); index++ {
		character := ascii.ToLower(value[index])
		if isSlugByte(character) {
			if separator && output.Len() > 0 {
				output.WriteByte('-')
			}
			separator = false
			output.WriteByte(character)
		} else {
			separator = true
		}
	}

	return strings.Trim(output.String(), "-")
}

// isSlugByte reports whether character can be preserved in a canonical page slug.
func isSlugByte(character byte) bool {
	return ascii.IsAlphanumeric(character) ||
		character == '/' || character == '_' || character == '-'
}
