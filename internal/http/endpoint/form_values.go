package endpoint

import (
	"errors"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/ascii"
)

var errMissingFormInteger = errors.New("integer form value is required")

// parseOptionalFormInt64 parses an optional base-10 integer, using zero for a blank value.
func parseOptionalFormInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	return strconv.ParseInt(value, 10, 64)
}

// parseRequiredPositiveFormInt64 parses a required positive base-10 integer.
func parseRequiredPositiveFormInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errMissingFormInteger
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, errMissingFormInteger
	}

	return parsed, nil
}

// parseOptionalFormInt parses an optional base-10 integer, using zero for a blank value.
func parseOptionalFormInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	return strconv.Atoi(value)
}

// validDynamicFormRow reports whether a dynamic form row identifier uses the bounded portable alphabet.
func validDynamicFormRow(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}

	for _, character := range value {
		if ascii.IsAlphanumeric(character) || character == '-' || character == '_' {
			continue
		}

		return false
	}

	return true
}
