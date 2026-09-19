package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// wikiJSPage contains the subset of a Wiki.js page export required by Kumbuka.
type wikiJSPage struct {
	// Path is the source page path from Wiki.js.
	Path string `json:"path"`
	// Title is the source page title from Wiki.js.
	Title string `json:"title"`
	// Content is the Markdown source exported by Wiki.js.
	Content string `json:"content"`
}

// importWikiJSON decodes pages from a Wiki.js JSON export.
func importWikiJSON(data []byte) ([]Candidate, error) {
	var pages []wikiJSPage
	if err := json.Unmarshal(data, &pages); err != nil {
		return nil, newValidationError(
			"The Wiki.js export contains invalid JSON.",
			fmt.Errorf("decode Wiki.js import: %w", err),
		)
	}
	if len(pages) == 0 {
		return nil, newValidationError(
			"The Wiki.js export must contain at least one page.",
			errors.New("validate Wiki.js export: no pages"),
		)
	}

	result := make([]Candidate, 0, len(pages))

	for index, page := range pages {
		page.Path = strings.TrimSpace(page.Path)
		page.Title = strings.TrimSpace(page.Title)

		if err := validateWikiJSPage(page, index); err != nil {
			return nil, err
		}

		result = append(result, Candidate{
			Slug:     page.Path,
			Title:    page.Title,
			Markdown: page.Content,
			Source:   "Wiki.js JSON",
		})
	}

	return result, nil
}

// validateWikiJSPage checks the required fields on one decoded Wiki.js page.
func validateWikiJSPage(page wikiJSPage, index int) error {
	pageNumber := index + 1

	switch {
	case page.Path == "":
		return newValidationError(
			fmt.Sprintf("Page %d from Wiki.js has no path.", pageNumber),
			fmt.Errorf("validate Wiki.js page %d: missing path", pageNumber),
		)
	case page.Title == "":
		return newValidationError(
			fmt.Sprintf("Page %d from Wiki.js has no title.", pageNumber),
			fmt.Errorf("validate Wiki.js page %d: missing title", pageNumber),
		)
	case page.Content == "":
		return newValidationError(
			fmt.Sprintf("Page %d from Wiki.js has no content.", pageNumber),
			fmt.Errorf("validate Wiki.js page %d: missing content", pageNumber),
		)
	default:
		return nil
	}
}
