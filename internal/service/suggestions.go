package service

import "strings"

// normalizeSuggestionText normalizes browser line endings and drops the textarea's final newline.
func normalizeSuggestionText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSuffix(value, "\n")
}
