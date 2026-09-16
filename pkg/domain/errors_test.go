package domain

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
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
