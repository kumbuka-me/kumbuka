// Package pagemove contains pure page-path rules shared by application workflows and persistence transactions.
package pagemove

import (
	"path"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// Normalize canonicalizes and validates one page move pair.
func Normalize(oldSlug, newSlug string, options domain.MovePageOptions) (string, string, error) {
	oldSlug = strings.Trim(strings.TrimSpace(oldSlug), "/")
	newSlug = md.Slug(newSlug)
	if oldSlug == "" || newSlug == "" {
		return "", "", domain.NewValidationError("slug", "A destination path is required.")
	}
	if oldSlug == newSlug {
		return "", "", domain.NewValidationError("slug", "Choose a different destination path.")
	}
	if options.MoveChildren && strings.HasPrefix(newSlug, oldSlug+"/") {
		return "", "", domain.NewValidationError("slug", "A page tree cannot be moved inside itself.")
	}
	return oldSlug, newSlug, nil
}

// NormalizeTarget canonicalizes and validates a bulk move target path.
func NormalizeTarget(target string) (string, error) {
	target = md.Slug(target)
	if target == "" {
		return "", domain.NewValidationError("target", "A target path is required.")
	}
	return target, nil
}

// Destination canonicalizes a bulk source path and returns its destination below target.
func Destination(source, target string) (string, string, error) {
	source = strings.Trim(strings.TrimSpace(source), "/")
	if source == "" {
		return "", "", domain.NewValidationError("pages", "Choose valid pages to move.")
	}
	destination := target + "/" + path.Base(source)
	if source == destination {
		return "", "", domain.NewValidationError("target", "Choose a different destination for every selected page.")
	}
	return source, destination, nil
}
