// Package ascii contains predicates for the ASCII character classes used by Kumbuka.
package ascii

// IsAlphanumeric reports whether character is an ASCII letter or digit.
func IsAlphanumeric(character rune) bool {
	return IsLetter(character) || IsDigit(character)
}

// IsLetter reports whether character is an ASCII letter.
func IsLetter(character rune) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z'
}

// IsDigit reports whether character is an ASCII decimal digit.
func IsDigit(character rune) bool {
	return character >= '0' && character <= '9'
}
