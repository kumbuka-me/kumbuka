// Package ascii contains predicates for the ASCII character classes used by Kumbuka.
package ascii

// IsAlphanumeric reports whether character is an ASCII letter or digit.
func IsAlphanumeric(character rune) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}
