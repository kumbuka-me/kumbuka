// Package route generates deployment-local browser URLs.
package route

import (
	"net/http"

	"github.com/containeroo/httpprefix"
)

// ForRequest generates a URL using the deployment prefix of a mounted request.
func ForRequest(r *http.Request, target string) string {
	return httpprefix.URLForRequest(r, target)
}

// Redirect uses the same local URL rules as templates.
func Redirect(w http.ResponseWriter, r *http.Request, target string, status int) {
	httpprefix.Redirect(w, r, target, status)
}
