package importer

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// MaxBytes is the maximum aggregate uncompressed content accepted by one import request.
const MaxBytes int64 = 100 << 20

// Format identifies one supported external page export format.
type Format string

const (
	// FormatMarkdown imports standalone Markdown files or ZIP archives containing Markdown.
	FormatMarkdown Format = "markdown"
	// FormatWikiJS imports Wiki.js JSON exports or ZIP archives containing them.
	FormatWikiJS Format = "wikijs"
	// FormatConfluence imports Confluence HTML exports or ZIP archives containing them.
	FormatConfluence Format = "confluence"
)

// Candidate contains one page discovered in an external import source.
type Candidate struct {
	// Slug is the source path that will be normalized by the page service.
	Slug string
	// Title is the human-readable page title discovered by the importer.
	Title string
	// Markdown is the canonical page source produced by the importer.
	Markdown string
	// Source describes the source format for revision history.
	Source string
}

// Budget tracks the aggregate uncompressed bytes consumed by an import request.
type Budget struct {
	// remaining is the number of uncompressed bytes that may still be imported.
	remaining int64
}

// validationError separates a safe user-facing explanation from its diagnostic cause.
type validationError struct {
	// message is safe to present to the user.
	message string
	// cause contains the internal diagnostic error.
	cause error
}

// NewBudget creates an import budget with the supplied byte limit.
func NewBudget(limit int64) *Budget {
	if limit < 0 {
		limit = 0
	}

	return &Budget{remaining: limit}
}

// Remaining returns the unconsumed import budget in bytes.
func (b *Budget) Remaining() int64 {
	if b == nil {
		return 0
	}

	return b.remaining
}

// consume subtracts bytes from the budget or returns a safe validation error.
func (b *Budget) consume(size int64, message string) error {
	if b == nil || size < 0 || size > b.remaining {
		return newValidationError(message, errors.New(strings.ToLower(message)))
	}

	b.remaining -= size
	return nil
}

// Error returns the internal diagnostic text for a validation failure.
func (e *validationError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}

	return "invalid import"
}

// Unwrap returns the diagnostic cause associated with a validation failure.
func (e *validationError) Unwrap() error {
	return e.cause
}

// UserMessage returns the validation text that is safe to present to a user.
func (e *validationError) UserMessage() string {
	return e.message
}

// newValidationError creates an importer validation failure with a safe user-facing message.
func newValidationError(message string, cause error) error {
	return &validationError{message: message, cause: cause}
}

// ParseFormat validates an explicitly selected import source format.
func ParseFormat(value string) (Format, error) {
	format := Format(strings.TrimSpace(value))

	switch format {
	case FormatMarkdown, FormatWikiJS, FormatConfluence:
		return format, nil
	default:
		return "", newValidationError(
			"Choose a source format.",
			fmt.Errorf("invalid import source format %q", value),
		)
	}
}

// ParseFile extracts page candidates from one uploaded file using the selected format.
func ParseFile(name string, source io.Reader, format Format, budget *Budget) ([]Candidate, error) {
	data, err := io.ReadAll(io.LimitReader(source, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxBytes {
		return nil, newValidationError("File exceeds 100 MiB.", errors.New("file exceeds 100 MiB"))
	}

	name = normalizeFilename(name)
	extension := strings.ToLower(path.Ext(name))

	if extension == ".zip" {
		return importZIP(data, format, budget)
	}
	if err := budget.consume(int64(len(data)), "Import contents exceed 100 MiB."); err != nil {
		return nil, err
	}

	switch format {
	case FormatMarkdown:
		return importMarkdownFile(name, extension, data)
	case FormatWikiJS:
		if extension != ".json" {
			return nil, newValidationError(
				"Imports from Wiki.js require .json or .zip files.",
				errors.New("validate Wiki.js import: unsupported file type"),
			)
		}

		return importWikiJSON(data)
	case FormatConfluence:
		return importConfluenceFile(name, extension, data)
	default:
		return nil, fmt.Errorf("unsupported source format %q", format)
	}
}

// normalizeFilename converts uploaded file names to slash-separated relative paths.
func normalizeFilename(name string) string {
	return strings.TrimPrefix(strings.ReplaceAll(name, "\\", "/"), "./")
}

// supportedExtension reports whether an archive entry belongs to the selected import format.
func supportedExtension(format Format, extension string) bool {
	switch format {
	case FormatMarkdown:
		return extension == ".md" || extension == ".markdown"
	case FormatWikiJS:
		return extension == ".json"
	case FormatConfluence:
		return extension == ".html" || extension == ".htm"
	default:
		return false
	}
}
