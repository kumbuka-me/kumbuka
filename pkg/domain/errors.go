package domain

import (
	"errors"
	"fmt"
)

// FieldError describes a safe, actionable input failure.
type FieldError struct {
	// Field identifies the invalid input field.
	Field string
	// Message contains the message associated with field error.
	Message string
}

// ValidationError carries input failures across persistence and application boundaries. Cause is optional diagnostic context and must not be included in HTTP responses.
type ValidationError struct {
	// Fields contains the fields associated with validation error.
	Fields []FieldError
	// Cause is the underlying validation failure.
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
	// Field identifies the invalid group-assignment input.
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

var (
	// ErrNotFound indicates that a requested domain object does not exist.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists indicates that a unique domain object already exists.
	ErrAlreadyExists = errors.New("already exists")
	// ErrForbidden indicates that a requested domain mutation is not permitted.
	ErrForbidden = errors.New("forbidden")
	// ErrRegistrationDisabled indicates that a new external identity may not create an account.
	ErrRegistrationDisabled = errors.New("user registration is disabled")
	// ErrIdentityApprovalRequired indicates that an OIDC identity awaits administrator approval.
	ErrIdentityApprovalRequired = errors.New("process OIDC identity: administrator approval is required")
	// ErrIdentityRejected indicates that an administrator rejected an OIDC identity.
	ErrIdentityRejected = errors.New("process OIDC identity: identity was rejected")
	// ErrPageInBin indicates that a page path is occupied by a recycled page.
	ErrPageInBin = errors.New("page path is in recycle bin")
	// ErrStaleReview indicates that a page changed after review was requested.
	ErrStaleReview = errors.New("page changed after review was requested")
	// ErrReviewPending indicates that a page already has a pending review request.
	ErrReviewPending = errors.New("review already pending")
	// ErrReviewClosed indicates that a completed review request cannot be changed.
	ErrReviewClosed = errors.New("review request is closed")
	// ErrReviewChangesRequired indicates that reviewer feedback must be addressed before another request.
	ErrReviewChangesRequired = errors.New("review changes must be addressed")
	// ErrReviewSuggestionConflict indicates that selected suggestions overlap and cannot be applied together.
	ErrReviewSuggestionConflict = errors.New("review suggestions overlap")
	// ErrStaleSuggestion indicates that an inline suggestion no longer targets the page revision it was created from.
	ErrStaleSuggestion = errors.New("page changed after suggestion was created")
)
