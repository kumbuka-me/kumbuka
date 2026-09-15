package auth

import "unicode/utf8"

const (
	// minimumLocalPasswordCharacters counts Unicode code points, not UTF-8 bytes.
	minimumLocalPasswordCharacters = 12
	// maximumLocalPasswordBytes is the input limit enforced by bcrypt.
	maximumLocalPasswordBytes = 72
)

// LocalPasswordProblem returns a user-facing validation message, or an empty string.
func LocalPasswordProblem(password string) string {
	if !utf8.ValidString(password) {
		return "Use valid UTF-8 characters."
	}
	if utf8.RuneCountInString(password) < minimumLocalPasswordCharacters {
		return "Use at least 12 characters."
	}
	if len(password) > maximumLocalPasswordBytes {
		return "Use at most 72 UTF-8 bytes."
	}
	return ""
}
