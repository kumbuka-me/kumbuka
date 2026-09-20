package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveDiscussionWriterStub captures page-bound discussion resolution from the HTTP adapter.
type resolveDiscussionWriterStub struct {
	pageDiscussionWriter
	// slug is the page path supplied by the handler.
	slug string
	// id is the comment identifier supplied by the handler.
	id int64
	// resolved is the requested resolution state.
	resolved bool
}

// ResolveComment captures one discussion resolution request.
func (s *resolveDiscussionWriterStub) ResolveComment(_ context.Context, slug string, id int64, resolved bool, _ domain.User) error {
	s.slug = slug
	s.id = id
	s.resolved = resolved

	return nil
}

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

// TestResolvePageCommentUsesRouteSlug verifies resolution cannot substitute a different page through form data.
func TestResolvePageCommentUsesRouteSlug(t *testing.T) {
	t.Parallel()

	writer := &resolveDiscussionWriterStub{}
	request := httptest.NewRequest(http.MethodPost, "/page-comments/resolve/42/docs/start", strings.NewReader("resolved=true&slug=other"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("id", "42")
	request.SetPathValue("slug", "docs/start")
	response := httptest.NewRecorder()

	ResolvePageComment(writer, nil).ServeHTTP(response, request)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "docs/start", writer.slug)
	assert.Equal(t, int64(42), writer.id)
	assert.True(t, writer.resolved)
}
