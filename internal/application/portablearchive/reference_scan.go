package portablearchive

import "github.com/kumbuka-me/kumbuka/pkg/ascii"

// portableResourceIDEnd returns the first byte after a decimal resource ID beginning at start.
func portableResourceIDEnd(value string, start int) int {
	end := start
	for end < len(value) && ascii.IsDigit(value[end]) {
		end++
	}
	return end
}

// validPortableResourceIDText reports whether value is a non-empty decimal resource ID.
func validPortableResourceIDText(value string) bool {
	return value != "" && portableResourceIDEnd(value, 0) == len(value)
}

// portableResourceReferenceEnd returns the first byte after a bare stored-resource reference suffix.
func portableResourceReferenceEnd(value string, start int) int {
	end := start
	for end < len(value) && !isPortableResourceReferenceTerminator(value[end]) {
		end++
	}
	return end
}

// isPortableResourceReferenceTerminator reports whether value ends a bare stored-resource URL in Markdown text.
func isPortableResourceReferenceTerminator(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\f', ')', '"', '\'':
		return true
	default:
		return false
	}
}
