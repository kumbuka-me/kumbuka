package route

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containeroo/httpprefix"
	"github.com/stretchr/testify/assert"
)

func TestRedirect(t *testing.T) {
	for _, prefix := range []string{"", "/kumbuka"} {
		handler := httpprefix.MountUnderPrefixWithOptions(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Redirect(w, r, "/pages/foo?view=history#changes", http.StatusSeeOther)
		}), prefix, httpprefix.WithRedirectRewriting())
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("POST", prefix+"/", nil))
		assert.Equal(t, http.StatusSeeOther, response.Code)
		assert.Equal(t, prefix+"/pages/foo?view=history#changes", response.Header().Get("Location"))
	}
}

func TestRewriteHTMLURLs(t *testing.T) {
	source := `<p><a href="/pages/foo">/pages/foo</a><img src="/media/7/a.png"><code>&lt;a href="/pages/foo"&gt;</code><a href="https://example.test/">external</a></p>`
	assert.Equal(t, source, RewriteHTMLURLs("", source))
	got := RewriteHTMLURLs("/kumbuka", source)
	assert.Contains(t, got, `href="/kumbuka/pages/foo"`)
	assert.Contains(t, got, `src="/kumbuka/media/7/a.png"`)
	assert.Contains(t, got, `<code>&lt;a href="/pages/foo"&gt;</code>`)
	assert.Contains(t, got, `href="https://example.test/"`)
}
