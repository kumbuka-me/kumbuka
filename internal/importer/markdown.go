package importer

import (
	"errors"
	"strings"
)

// importMarkdownFile converts one standalone Markdown upload into a page candidate.
func importMarkdownFile(name, extension string, data []byte) ([]Candidate, error) {
	if extension != ".md" && extension != ".markdown" {
		return nil, newValidationError(
			"Markdown imports require .md, .markdown, or .zip files.",
			errors.New("markdown import has unsupported file type"),
		)
	}

	return markdownCandidate(name, extension, data, "Markdown")
}

// importMarkdownArchiveEntry converts one Markdown archive entry into a page candidate.
func importMarkdownArchiveEntry(name, extension string, data []byte) ([]Candidate, error) {
	return markdownCandidate(name, extension, data, "ZIP/Markdown")
}

// markdownCandidate creates a page candidate after discovering its required level-one title.
func markdownCandidate(name, extension string, data []byte, source string) ([]Candidate, error) {
	title, err := markdownTitle(string(data))
	if err != nil {
		return nil, err
	}

	return []Candidate{{
		Slug:     strings.TrimSuffix(name, extension),
		Title:    title,
		Markdown: string(data),
		Source:   source,
	}}, nil
}

// markdownTitle returns the first level-one heading in a Markdown document.
func markdownTitle(markdown string) (string, error) {
	for line := range strings.SplitSeq(markdown, "\n") {
		heading, ok := strings.CutPrefix(strings.TrimSpace(line), "# ")
		if !ok {
			continue
		}

		heading = strings.TrimSpace(heading)
		if heading != "" {
			return heading, nil
		}
	}

	return "", newValidationError(
		"Document requires a level-one Markdown heading for its title.",
		errors.New("document requires a level-one Markdown heading for its title"),
	)
}
