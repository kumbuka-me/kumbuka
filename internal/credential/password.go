// Package credential contains reusable local-credential policy and hashing primitives.
package credential

import (
	"unicode/utf8"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"golang.org/x/crypto/bcrypt"
)

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

// HashLocalPassword validates and hashes one local password with bcrypt.
func HashLocalPassword(password string) (string, error) {
	if problem := LocalPasswordProblem(password); problem != "" {
		return "", domain.NewValidationError("password", problem)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// Passwords adapts local password policy and hashing for application use cases.
type Passwords struct{}

// Problem returns a user-facing password validation message, or an empty string.
func (Passwords) Problem(password string) string {
	return LocalPasswordProblem(password)
}

// Hash validates and hashes one local password.
func (Passwords) Hash(password string) (string, error) {
	return HashLocalPassword(password)
}
