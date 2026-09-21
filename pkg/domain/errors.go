package domain

import "fmt"

// FieldError describes a safe, actionable input failure.
type FieldError struct {
	// Field stores the field value used by field error.
	Field string
	// Message contains the message associated with field error.
	Message string
}

// ValidationError carries input failures across persistence and application boundaries. Cause is optional diagnostic context and must not be included in HTTP responses.
type ValidationError struct {
	// Fields contains the fields associated with validation error.
	Fields []FieldError
	// Cause stores the cause value used by validation error.
	Cause error
}

// Error returns the validation failure summary and diagnostic cause when present.
func (e *ValidationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("validation failed: %v", e.Cause)
	}

	return "validation failed"
}

// UserMessage returns the first explicitly safe field message, when present.
func (e *ValidationError) UserMessage() string {
	if len(e.Fields) == 0 {
		return ""
	}

	return e.Fields[0].Message
}

// Unwrap preserves a persistence cause for errors.Is and errors.As.
func (e *ValidationError) Unwrap() error { return e.Cause }

// NewValidationError creates a single-field input failure.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: message}}}
}

// GroupAssignmentError identifies which page group selection is not assignable. It deliberately does not distinguish a hidden group from a nonexistent group.
type GroupAssignmentError struct {
	// Field stores the field value used by group assignment error.
	Field string
}

// Error returns the stable forbidden-assignment message.
func (e *GroupAssignmentError) Error() string { return "page group assignment is forbidden" }

// Unwrap classifies a group assignment failure as forbidden.
func (e *GroupAssignmentError) Unwrap() error { return ErrForbidden }

// Specific missing-resource errors also classify as ErrNotFound.
var (
	ErrRevisionNotFound = fmt.Errorf("revision: %w", ErrNotFound)
	ErrCommentNotFound  = fmt.Errorf("comment: %w", ErrNotFound)
)
