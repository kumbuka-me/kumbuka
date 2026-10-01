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
