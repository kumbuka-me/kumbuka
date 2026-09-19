package importer

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// importZIP extracts supported page candidates from one ZIP archive.
func importZIP(data []byte, format Format, budget *Budget) ([]Candidate, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, newValidationError(
			"The ZIP archive is invalid.",
			fmt.Errorf("open ZIP archive: %w", err),
		)
	}

	var result []Candidate

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}

		name := normalizeFilename(entry.Name)
		extension := strings.ToLower(path.Ext(name))
		if !supportedExtension(format, extension) {
			continue
		}
		if budget == nil || entry.UncompressedSize64 > uint64(budget.Remaining()) {
			return nil, newValidationError(
				"Archive contents exceed 100 MiB.",
				errors.New("archive contents exceed 100 MiB"),
			)
		}

		content, err := readArchiveEntry(entry, budget.Remaining())
		if err != nil {
			return nil, wrapEntryError(name, err)
		}
		if err := budget.consume(int64(len(content)), "Archive contents exceed 100 MiB."); err != nil {
			return nil, wrapEntryError(name, err)
		}

		items, err := importArchiveEntry(name, extension, content, format)
		if err != nil {
			return nil, wrapEntryError(name, err)
		}

		result = append(result, items...)
	}

	return result, nil
}

// importArchiveEntry converts one supported ZIP entry into page candidates.
func importArchiveEntry(name, extension string, content []byte, format Format) ([]Candidate, error) {
	switch format {
	case FormatMarkdown:
		return importMarkdownArchiveEntry(name, extension, content)
	case FormatWikiJS:
		return importWikiJSON(content)
	case FormatConfluence:
		return importConfluenceArchiveEntry(name, extension, content)
	default:
		return nil, fmt.Errorf("unsupported source format %q", format)
	}
}

// readArchiveEntry reads and closes one ZIP entry without exceeding the supplied byte limit.
func readArchiveEntry(entry *zip.File, remaining int64) ([]byte, error) {
	file, err := entry.Open()
	if err != nil {
		return nil, err
	}

	content, readErr := io.ReadAll(io.LimitReader(file, remaining+1))
	closeErr := file.Close()

	switch {
	case readErr != nil:
		return nil, readErr
	case closeErr != nil:
		return nil, closeErr
	case int64(len(content)) > remaining:
		return nil, newValidationError(
			"Archive contents exceed 100 MiB.",
			errors.New("archive contents exceed 100 MiB"),
		)
	default:
		return content, nil
	}
}

// wrapEntryError adds the archive entry name while preserving safe importer validation text.
func wrapEntryError(name string, err error) error {
	diagnostic := fmt.Errorf("%s: %w", name, err)

	message, ok := safeMessage(err)
	if !ok {
		return diagnostic
	}

	return newValidationError(name+": "+message, diagnostic)
}

// safeMessage extracts a non-empty user-facing message from an importer error.
func safeMessage(err error) (string, bool) {
	type userMessageError interface {
		error
		UserMessage() string
	}

	userErr, ok := errors.AsType[userMessageError](err)
	if !ok {
		return "", false
	}

	message := strings.TrimSpace(userErr.UserMessage())
	return message, message != ""
}
