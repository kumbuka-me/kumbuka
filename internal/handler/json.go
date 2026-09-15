package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxJSONRequestBytes = 2 << 20

// decode reads exactly one non-null, size-limited JSON value and rejects unknown fields.
func decode[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var zero T
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	// A pointer distinguishes JSON null from the zero value of a request struct.
	var value *T
	if err := decoder.Decode(&value); err != nil {
		return zero, newJSONRequestError(err)
	}
	if value == nil {
		return zero, newRequestError(
			"",
			"JSON null is not allowed.",
			errors.New("invalid JSON request: null is not allowed"),
		)
	}

	// Decode reads one value, not the complete body. Require EOF to reject a
	// second value, trailing junk, and oversized whitespace after valid JSON.
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		if err == nil {
			return zero, newRequestError(
				"",
				"Only one JSON value is allowed.",
				errors.New("invalid JSON request: only one value is allowed"),
			)
		}

		return zero, newJSONRequestError(err)
	}

	return *value, nil
}

// newJSONRequestError preserves decoder diagnostics while marking a safe presentation message.
func newJSONRequestError(err error) error {
	internal := fmt.Errorf("invalid JSON request: %w", err)

	if errors.Is(err, io.EOF) {
		return newRequestError("", "Request body must contain JSON.", internal)
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return newRequestError("", "Request body contains an unknown field.", internal)
	}
	if _, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return newRequestError("", "A request field has the wrong type.", internal)
	}
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return newRequestError("", "Request body must not exceed 2 MiB.", internal)
	}

	return newRequestError("", "Request body contains invalid JSON.", internal)
}

// jsonSlice keeps collection responses as arrays even when a service returns nil.
func jsonSlice[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
