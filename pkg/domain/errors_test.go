package domain

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationErrorSeparatesDiagnosticAndUserMessages(t *testing.T) {
	t.Parallel()

	err := NewValidationError("icon", "Choose an icon from the available icon catalog.")

	assert.Equal(t, "validation failed", err.Error())
	assert.Equal(t, "Choose an icon from the available icon catalog.", err.UserMessage())
}

func TestValidationErrorPreservesCauseWithoutExposingItAsUserMessage(t *testing.T) {
	t.Parallel()

	cause := errors.New("private database detail")
	err := NewValidationError("name", "A name is required.")
	err.Cause = cause

	assert.Equal(t, "validation failed: private database detail", err.Error())
	assert.Equal(t, "A name is required.", err.UserMessage())
	assert.ErrorIs(t, err, cause)
}

func TestValidationErrorWithoutFieldsDoesNotExposeCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("private database detail")
	err := &ValidationError{Cause: cause}
	assert.Empty(t, err.UserMessage())
	assert.ErrorIs(t, err, cause)
}

func TestValidationErrorUsesFirstFieldMessage(t *testing.T) {
	t.Parallel()
	err := &ValidationError{Fields: []FieldError{{Field: "name", Message: "Enter a name."}, {Field: "path", Message: "Enter a path."}}}
	assert.Equal(t, "Enter a name.", err.UserMessage())
	assert.Nil(t, err.Unwrap())
}

func TestGroupAssignmentErrorIsForbidden(t *testing.T) {
	t.Parallel()
	assignment := &GroupAssignmentError{Field: "groups"}
	wrapped := fmt.Errorf("save page: %w", assignment)
	require.ErrorIs(t, wrapped, ErrForbidden)
	var extracted *GroupAssignmentError
	require.ErrorAs(t, wrapped, &extracted)
	assert.Equal(t, "groups", extracted.Field)
	assert.Equal(t, "page group assignment is forbidden", assignment.Error())
}

func TestRevisionNotFoundClassification(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, ErrRevisionNotFound, ErrNotFound)
	assert.NotErrorIs(t, ErrRevisionNotFound, ErrCommentNotFound)
}

func TestCommentNotFoundClassification(t *testing.T) {
	t.Parallel()
	assert.ErrorIs(t, ErrCommentNotFound, ErrNotFound)
	assert.NotErrorIs(t, ErrCommentNotFound, ErrRevisionNotFound)
}
