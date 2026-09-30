package markdown

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kumbuka-me/kumbuka/pkg/ascii"
)

// Slug converts human-readable page text into a canonical page slug.
func Slug(value string) string {
	value = strings.TrimSpace(value)
	var output strings.Builder
	output.Grow(len(value))
	separator := false

	for index := 0; index < len(value); {
		if ascii.IsASCII(value[index]) {
			character := ascii.ToLower(value[index])
			index++
			separator = writeSlugByte(&output, character, separator)
			continue
		}

		character, size := utf8.DecodeRuneInString(value[index:])
		index += size
		character = unicode.ToLower(character)
		separator = writeSlugRune(&output, character, separator)
	}

	return strings.Trim(output.String(), "-")
}

// writeSlugByte appends one normalized ASCII slug character and reports whether a separator remains pending.
func writeSlugByte(output *strings.Builder, character byte, separator bool) bool {
	if !isSlugByte(character) {
		return true
	}
	writePendingSlugSeparator(output, separator)
	output.WriteByte(character)
	return false
}

// writePendingSlugSeparator writes one separator between preserved slug components when needed.
func writePendingSlugSeparator(output *strings.Builder, pending bool) {
	if pending && output.Len() > 0 {
		output.WriteByte('-')
	}
}

// writeSlugRune appends one Unicode-lowered rune when it maps into Kumbuka's ASCII slug alphabet.
func writeSlugRune(output *strings.Builder, character rune, separator bool) bool {
	if character > unicode.MaxASCII {
		return true
	}
	return writeSlugByte(output, byte(character), separator)
}

// isSlugByte reports whether character can be preserved in a canonical page slug.
func isSlugByte(character byte) bool {
	return ascii.IsAlphanumeric(character) ||
		character == '/' || character == '_' || character == '-'
}
