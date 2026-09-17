package flags

// ToPtr returns a pointer to value.
func ToPtr[T any](value T) *T {
	return &value
}
