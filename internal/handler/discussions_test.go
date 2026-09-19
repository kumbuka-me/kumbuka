package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPageCommentReturnTarget verifies page-level and inline discussion redirects stay local and contextual.
func TestPageCommentReturnTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{name: "page discussion", value: "/pages/docs/start", expected: "/pages/docs/start#comments"},
		{name: "inline discussion", value: "/pages/docs/start#comment-42", expected: "/pages/docs/start#comment-42"},
		{name: "external target", value: "https://example.test/pages/start", expected: "/"},
		{name: "protocol relative target", value: "//example.test/pages/start", expected: "/"},
		{name: "non page target", value: "/admin", expected: "/"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.expected, pageCommentReturnTarget(test.value))
		})
	}
}
