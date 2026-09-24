package portable

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// ValidationError reports a safe archive-format problem and retains its diagnostic cause.
type ValidationError struct {
	// Message is safe to show to an administrator.
	Message string
	// Cause retains the diagnostic validation failure.
	Cause error
}

// Error returns the safe archive validation message.
func (e *ValidationError) Error() string { return e.Message }

// Unwrap exposes the diagnostic cause for errors.Is and errors.As.
func (e *ValidationError) Unwrap() error { return e.Cause }

// Detect identifies a portable manifest within the caller's uncompressed-size limit.
func Detect(data []byte, maxUncompressedBytes int64) bool {
	if maxUncompressedBytes <= 0 {
		return false
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return false
	}

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || entry.Name != ManifestPath {
			continue
		}

		remaining := maxUncompressedBytes
		data, err := readZipFile(entry, &remaining)
		if err != nil {
			return false
		}

		var manifest struct {
			// Format identifies the producer without decoding the page inventory.
			Format string `json:"format"`
		}
		if json.Unmarshal(data, &manifest) != nil {
			return false
		}
		return manifest.Format == Format
	}

	return false
}

// Archive is a fully validated portable archive held in memory before mutations begin.
type Archive struct {
	// Manifest is the decoded root archive inventory.
	Manifest Manifest
	// Pages contains validated page source and metadata in manifest order.
	Pages []Page
	// Resources maps manifest resource paths to validated binary payloads.
	Resources map[string][]byte
}

// Page groups one manifest page entry with its decoded portable contents.
type Page struct {
	// Entry identifies the page's source and metadata paths.
	Entry PageEntry
	// Metadata contains the portable page settings.
	Metadata PageMetadata
	// Markdown is the page source with archive-relative resource references.
	Markdown string
}

// Parse validates a Kumbuka archive completely before any data is written.
func Parse(data []byte, maxUncompressedBytes int64) (Archive, error) {
	if maxUncompressedBytes <= 0 {
		return Archive{}, validationError("Kumbuka archive size limit is invalid.", errors.New("portable archive size limit must be positive"))
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Archive{}, validationError("The Kumbuka ZIP archive is invalid.", err)
	}
	entries, err := indexArchiveEntries(reader.File)
	if err != nil {
		return Archive{}, err
	}

	remaining := maxUncompressedBytes
	manifest, err := readPortableManifest(entries, &remaining)
	if err != nil {
		return Archive{}, err
	}
	contents := Archive{Manifest: manifest, Pages: make([]Page, 0, len(manifest.Pages)), Resources: map[string][]byte{}}
	used := map[string]bool{ManifestPath: true}

	for _, pageEntry := range manifest.Pages {
		pageData, err := readArchivePage(entries, pageEntry, used, &remaining)
		if err != nil {
			return Archive{}, err
		}
		contents.Pages = append(contents.Pages, pageData)
	}
	if err := readArchiveResources(entries, manifest.Media, "media/", used, &remaining, contents.Resources); err != nil {
		return Archive{}, err
	}
	if err := readArchiveResources(entries, manifest.Attachments, "attachments/", used, &remaining, contents.Resources); err != nil {
		return Archive{}, err
	}
	if len(used) != len(entries) {
		return Archive{}, validationError(
			"The Kumbuka archive contains files that are not listed in its manifest.",
			errors.New("portable archive contains unlisted files"),
		)
	}
	return contents, nil
}

// indexArchiveEntries validates archive paths and rejects duplicate file entries.
func indexArchiveEntries(files []*zip.File) (map[string]*zip.File, error) {
	entries := make(map[string]*zip.File, len(files))
	for _, entry := range files {
		if entry.FileInfo().IsDir() {
			continue
		}
		name, err := validArchivePath(entry.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := entries[name]; exists {
			return nil, validationError(
				"The Kumbuka archive contains duplicate paths.",
				fmt.Errorf("duplicate archive path %q", name),
			)
		}
		entries[name] = entry
	}
	return entries, nil
}

// readPortableManifest loads and validates the required portable archive manifest.
func readPortableManifest(entries map[string]*zip.File, remaining *int64) (Manifest, error) {
	manifestEntry, ok := entries[ManifestPath]
	if !ok {
		return Manifest{}, validationError(
			"The Kumbuka archive is missing manifest.json.",
			errors.New("portable archive manifest is missing"),
		)
	}
	manifestData, err := readZipFile(manifestEntry, remaining)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return Manifest{}, validationError(
			"The Kumbuka archive manifest contains invalid JSON.",
			fmt.Errorf("decode portable manifest: %w", err),
		)
	}
	if err := validatePortableManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// validatePortableManifest validates format, version, and required page inventory.
func validatePortableManifest(manifest Manifest) error {
	if manifest.Format != Format {
		return validationError(
			"The ZIP is not a Kumbuka portable archive.",
			fmt.Errorf("unexpected portable archive format %q", manifest.Format),
		)
	}
	if manifest.Version != Version {
		return validationError(
			fmt.Sprintf("Kumbuka archive version %d is not supported by this build.", manifest.Version),
			fmt.Errorf("unsupported portable archive version %d", manifest.Version),
		)
	}
	if len(manifest.Pages) == 0 {
		return validationError(
			"The Kumbuka archive contains no pages.",
			errors.New("portable archive contains no pages"),
		)
	}
	return nil
}

// readArchiveResources loads one manifest resource collection into the archive result.
func readArchiveResources(entries map[string]*zip.File, resources []ResourceEntry, prefix string, used map[string]bool, remaining *int64, contents map[string][]byte) error {
	for _, resource := range resources {
		if err := readArchiveResource(entries, resource, prefix, used, remaining, contents); err != nil {
			return err
		}
	}
	return nil
}

// readArchivePage validates and reads one page and its metadata sidecar.
func readArchivePage(
	entries map[string]*zip.File,
	pageEntry PageEntry,
	used map[string]bool,
	remaining *int64,
) (Page, error) {
	slug := strings.TrimSpace(pageEntry.Slug)
	if !validSlug(slug) {
		return Page{}, validationError(
			"The Kumbuka archive contains an invalid page path.",
			fmt.Errorf("invalid portable page slug %q", pageEntry.Slug),
		)
	}

	expectedMarkdown := path.Join("pages", slug+".md")
	expectedMetadata := path.Join("metadata", slug+".json")
	if pageEntry.Markdown != expectedMarkdown || pageEntry.Metadata != expectedMetadata {
		return Page{}, validationError(
			"The Kumbuka archive page inventory is inconsistent.",
			fmt.Errorf("page %q paths do not match canonical paths", slug),
		)
	}

	markdown, err := readManifestFile(entries, pageEntry.Markdown, used, remaining)
	if err != nil {
		return Page{}, err
	}
	metadataData, err := readManifestFile(entries, pageEntry.Metadata, used, remaining)
	if err != nil {
		return Page{}, err
	}

	var metadata PageMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		return Page{}, validationError(
			"The Kumbuka archive contains invalid page metadata.",
			fmt.Errorf("decode metadata for %q: %w", slug, err),
		)
	}
	if metadata.Slug != slug || strings.TrimSpace(metadata.Title) == "" {
		return Page{}, validationError(
			"The Kumbuka archive contains inconsistent page metadata.",
			fmt.Errorf("metadata for %q has slug %q or empty title", slug, metadata.Slug),
		)
	}
	if !validPortableWorkflowMetadata(metadata) {
		return Page{}, validationError(
			"The Kumbuka archive contains invalid page workflow metadata.",
			fmt.Errorf("invalid workflow metadata for %q", slug),
		)
	}
	if err := validateGroupNames(metadata); err != nil {
		return Page{}, err
	}

	return Page{
		Entry:    pageEntry,
		Metadata: metadata,
		Markdown: string(markdown),
	}, nil
}

// readArchiveResource validates and reads one manifest resource entry.
func readArchiveResource(
	entries map[string]*zip.File,
	resource ResourceEntry,
	prefix string,
	used map[string]bool,
	remaining *int64,
	resources map[string][]byte,
) error {
	if !validArchiveResource(resource, prefix) {
		return validationError(
			"The Kumbuka archive contains an invalid resource inventory.",
			fmt.Errorf("invalid portable resource %q", resource.Path),
		)
	}
	if used[resource.Path] {
		return validationError(
			"The Kumbuka archive contains duplicate resource entries.",
			fmt.Errorf("duplicate portable resource %q", resource.Path),
		)
	}

	data, err := readManifestFile(entries, resource.Path, used, remaining)
	if err != nil {
		return err
	}
	resources[resource.Path] = data
	return nil
}

// validArchiveResource reports whether a manifest resource stays in its section and names its own path.
func validArchiveResource(resource ResourceEntry, prefix string) bool {
	return strings.HasPrefix(resource.Path, prefix) &&
		strings.TrimSpace(resource.Filename) != "" &&
		path.Base(resource.Path) == resource.Filename
}

// readManifestFile reads one manifest-listed path and marks it consumed.
func readManifestFile(
	entries map[string]*zip.File,
	name string,
	used map[string]bool,
	remaining *int64,
) ([]byte, error) {
	if used[name] {
		return nil, validationError(
			"The Kumbuka archive contains duplicate manifest references.",
			fmt.Errorf("duplicate manifest path %q", name),
		)
	}
	entry, ok := entries[name]
	if !ok {
		return nil, validationError(
			"The Kumbuka archive is missing a file listed in its manifest.",
			fmt.Errorf("missing manifest path %q", name),
		)
	}

	data, err := readZipFile(entry, remaining)
	if err != nil {
		return nil, err
	}
	used[name] = true
	return data, nil
}

// readZipFile reads one ZIP entry without exceeding the shared uncompressed budget.
func readZipFile(entry *zip.File, remaining *int64) ([]byte, error) {
	if entry.UncompressedSize64 > uint64(*remaining) {
		return nil, validationError(
			"Kumbuka archive contents exceed 100 MiB.",
			errors.New("portable archive contents exceed 100 MiB"),
		)
	}

	file, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(file, *remaining+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > *remaining {
		return nil, validationError(
			"Kumbuka archive contents exceed 100 MiB.",
			errors.New("portable archive contents exceed 100 MiB"),
		)
	}
	*remaining -= int64(len(data))
	return data, nil
}

// validPortableWorkflowMetadata reports whether imported page workflow metadata uses supported values.
func validPortableWorkflowMetadata(metadata PageMetadata) bool {
	return domain.ValidPageStatus(domain.PageStatus(metadata.Status)) && domain.ValidReviewIntervalDays(metadata.ReviewIntervalDays)
}

// validArchivePath normalizes one ZIP path and rejects traversal or platform-specific separators.
func validArchivePath(name string) (string, error) {
	if invalidArchivePathShape(name) {
		return "", validationError(
			"The Kumbuka archive contains an invalid file path.",
			fmt.Errorf("invalid archive path %q", name),
		)
	}
	clean := path.Clean(name)
	if unsafeArchivePath(name, clean) {
		return "", validationError(
			"The Kumbuka archive contains an unsafe file path.",
			fmt.Errorf("unsafe archive path %q", name),
		)
	}
	return clean, nil
}

// invalidArchivePathShape reports whether a ZIP path is empty, platform-specific, or absolute.
func invalidArchivePathShape(name string) bool {
	return name == "" || strings.Contains(name, "\\") || path.IsAbs(name)
}

// unsafeArchivePath reports whether normalization changes a ZIP path or reveals traversal.
func unsafeArchivePath(name, clean string) bool {
	return clean != name || clean == "." || clean == ".." || strings.HasPrefix(clean, "../")
}

// validSlug reports whether slug is already a canonical relative page path.
func validSlug(slug string) bool {
	return slug != "" && slug != "." && md.Slug(slug) == slug && !strings.HasPrefix(slug, "/") &&
		!strings.HasSuffix(slug, "/") && !strings.Contains(slug, "//") && path.Clean(slug) == slug
}

// validateGroupNames rejects blank collaboration group references before any groups are created.
func validateGroupNames(metadata PageMetadata) error {
	for _, name := range append(slices.Clone(metadata.Groups), metadata.OwnerGroup) {
		if name != "" && strings.TrimSpace(name) == "" {
			return validationError(
				"The Kumbuka archive contains an invalid collaboration group name.",
				errors.New("portable archive contains blank group name"),
			)
		}
	}
	return nil
}

// validationError creates one typed safe archive validation failure.
func validationError(message string, cause error) error {
	return &ValidationError{Message: message, Cause: cause}
}
