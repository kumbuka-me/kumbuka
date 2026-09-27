// Package utils provides small, dependency-free helpers shared across Kumbuka.
package utils

// ToPtr returns a pointer to value.
func ToPtr[T any](value T) *T {
	return &value
}
