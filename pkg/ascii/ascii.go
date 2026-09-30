// Package ascii contains predicates and transforms for the ASCII character classes used by Kumbuka.
package ascii

// character is an ASCII-compatible code-unit type used by byte and rune scanners.
type character interface {
	~byte | ~rune
}

// IsAlphanumeric reports whether character is an ASCII letter or digit.
func IsAlphanumeric[T character](character T) bool {
	return IsLetter(character) || IsDigit(character)
}

// IsLetter reports whether character is an ASCII letter.
func IsLetter[T character](character T) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z'
}

// IsDigit reports whether character is an ASCII decimal digit.
func IsDigit[T character](character T) bool {
	return character >= '0' && character <= '9'
}

// IsHexDigit reports whether character is an ASCII hexadecimal digit.
func IsHexDigit[T character](character T) bool {
	return IsDigit(character) ||
		character >= 'a' && character <= 'f' ||
		character >= 'A' && character <= 'F'
}

// ToLower returns the ASCII lowercase form of character.
func ToLower[T character](character T) T {
	if character >= 'A' && character <= 'Z' {
		return character + ('a' - 'A')
	}
	return character
}
