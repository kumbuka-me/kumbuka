package endpoint

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/sdk/renderartifact"
	"github.com/stretchr/testify/require"
)

// renderArtifactPageStub controls authorization independently of SVG storage.
type renderArtifactPageStub struct {
	// err rejects access to the owning page when non-nil.
	err error
}

// PageByIDFor simulates the page authorization boundary.
func (s renderArtifactPageStub) PageByIDFor(context.Context, domain.User, int64) (domain.Page, error) {
	return domain.Page{}, s.err
}

// renderArtifactStoreStub is a passive in-memory response for a single page artifact.
type renderArtifactStoreStub struct {
	// content is returned only after page authorization.
	content []byte
	// found reports whether the requested artifact exists.
	found bool
}

// PutPageSVG implements the page-scoped artifact storage contract.
func (s renderArtifactStoreStub) PutPageSVG(context.Context, int64, time.Time, []byte) (string, error) {
	return "", errors.New("not used by the read handler")
}

// GetPageSVG returns a previously stored image for the test.
func (s renderArtifactStoreStub) GetPageSVG(context.Context, int64, string) ([]byte, bool, error) {
	return s.content, s.found, nil
}

// TestPageRenderArtifact validates authorization and passive image response headers.
func TestPageRenderArtifact(t *testing.T) {
	t.Parallel()

	t.Run("serves an authorized SVG", func(t *testing.T) {
		t.Parallel()

		image := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect width="20" height="30"/></svg>`)
		digest := renderartifact.Digest(image)
		request := httptest.NewRequest(http.MethodGet, "/api/render-artifacts/7/"+digest+".svg", nil)
		request.SetPathValue("pageID", "7")
		request.SetPathValue("digest", digest+".svg")
		response := httptest.NewRecorder()

		PageRenderArtifact(renderArtifactPageStub{}, renderArtifactStoreStub{content: image, found: true})(response, request)

		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "image/svg+xml", response.Header().Get("Content-Type"))
		require.Contains(t, response.Header().Get("Content-Security-Policy"), "sandbox")
		require.Equal(t, string(image), response.Body.String())
	})

	t.Run("denies an inaccessible page", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodGet, "/api/render-artifacts/7/"+renderartifact.Digest([]byte("test"))+".svg", nil)
		request.SetPathValue("pageID", "7")
		request.SetPathValue("digest", renderartifact.Digest([]byte("test"))+".svg")
		response := httptest.NewRecorder()

		PageRenderArtifact(renderArtifactPageStub{err: domain.ErrNotFound}, renderArtifactStoreStub{found: true})(response, request)

		require.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("rejects invalid digests", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodGet, "/api/render-artifacts/7/bad.svg", nil)
		request.SetPathValue("pageID", "7")
		request.SetPathValue("digest", "bad.svg")
		response := httptest.NewRecorder()

		PageRenderArtifact(renderArtifactPageStub{}, renderArtifactStoreStub{found: true})(response, request)

		require.Equal(t, http.StatusNotFound, response.Code)
	})
}
