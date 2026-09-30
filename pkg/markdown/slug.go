package markdown

import (
	"strings"
)

// Slug converts human-readable page text into a canonical page slug.
func Slug(value string) string {
	value = strings.TrimSpace(value)
	var output strings.Builder
	output.Grow(len(value))
	separator := false

	for index := 0; index < len(value); index++ {
		character := value[index]
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
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
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' ||
		character == '/' || character == '_' || character == '-'
}
