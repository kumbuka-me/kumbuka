// Package pageurl constructs canonical application-local page URLs.
package pageurl

import (
	"strconv"
	"strings"
)

// Page returns the canonical application-local URL for a persisted page.
func Page(id int64, slug string) string {
	path := "/p/" + strconv.FormatInt(id, 10)
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return path
	}
	return path + "/" + slug
}
