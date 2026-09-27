package webview

import (
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrefixRendering(t *testing.T) {
	for _, prefix := range []string{"", "/kumbuka"} {
		t.Run(prefix, func(t *testing.T) {
			views, err := New(web.Assets, testViewsLogger(), "test", "test", nil, RuntimeInfo{RoutePrefix: prefix})
			require.NoError(t, err)
			layout, err := views.PublicData("Login")
			require.NoError(t, err)
			assert.Equal(t, prefix, layout.RoutePrefix)
			response := httptest.NewRecorder()
			views.RenderPublic(response, "login", AuthenticationView{Layout: layout})
			body := response.Body.String()
			assert.Contains(t, body, `data-route-prefix="`+prefix+`"`)
			assert.Contains(t, body, `action="`+prefix+`/auth/local"`)
			assert.Contains(t, body, `href="`+prefix+`/assets/v-`)
			assert.Contains(t, body, `src="`+prefix+`/brand/logo"`)
			assert.NotContains(t, body, "ZgotmplZ")
			assert.Equal(t, prefix+"/pages/foo", views.Route("/pages/foo"))
		})
	}
}
