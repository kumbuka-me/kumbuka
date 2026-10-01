// Package route generates deployment-local browser URLs.
package route

import (
	"net/http"
	"strings"

	"github.com/containeroo/httpprefix"
)

// ForRequest generates a URL using the deployment prefix of a mounted request.
func ForRequest(r *http.Request, target string) string {
	return httpprefix.URLForRequest(r, target)
}

// PrefixForRequest returns the normalized deployment prefix of a mounted request.
func PrefixForRequest(r *http.Request) string {
	return strings.TrimSuffix(ForRequest(r, "/"), "/")
}

// Redirect uses the same local URL rules as templates.
func Redirect(w http.ResponseWriter, r *http.Request, target string, status int) {
	httpprefix.Redirect(w, r, target, status)
}
