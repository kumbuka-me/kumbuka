package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/portable"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// portableArchiveImportMediaService exposes only the binary writes required by portable import.
type portableArchiveImportMediaService interface {
	UploadImage(context.Context, string, []byte, domain.User) (domain.Image, error)
	UploadAttachment(context.Context, string, []byte, domain.User) (domain.Attachment, error)
}

// portableArchivePageImportService combines legacy imports with portable archive page restoration.
type portableArchivePageImportService interface {
	pageImportService
	ImportPortable(context.Context, []service.PortableImportedPage, domain.User) (int, error)
}

// portableArchiveGroupService resolves and creates collaboration groups referenced by an archive.
type portableArchiveGroupService interface {
	Groups(context.Context) ([]domain.Group, error)
	CreateGroup(context.Context, string) (domain.Group, error)
}

// portableArchiveContents is a fully validated archive held in memory before mutations begin.
type portableArchiveContents struct {
	// Manifest is the decoded root archive inventory.
	Manifest portable.Manifest
	// Pages contains validated page source and metadata in manifest order.
	Pages []portableArchivePage
	// Resources maps manifest resource paths to validated binary payloads.
	Resources map[string][]byte
}

// portableArchivePage groups one manifest page entry with its decoded portable contents.
type portableArchivePage struct {
	// Entry identifies the page's source and metadata paths.
	Entry portable.PageEntry
	// Metadata contains the portable page settings.
	Metadata portable.PageMetadata
	// Markdown is the page source with archive-relative resource references.
	Markdown string
}

// ImportPagesWithPortableArchive extends the normal admin importer with Kumbuka portable archives.
func ImportPagesWithPortableArchive(
	pageUseCases portableArchivePageImportService,
	mediaUseCases portableArchiveImportMediaService,
	groupUseCases portableArchiveGroupService,
	logger *slog.Logger,
) http.HandlerFunc {
	legacy := ImportPages(pageUseCases, logger)

	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
		if err := r.ParseMultipartForm(maxImportBytes); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Import is too large or invalid.")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll() // nolint:errcheck
		}

		if strings.TrimSpace(r.FormValue("format")) != portable.Format {
			legacy.ServeHTTP(w, r)
			return
		}

		headers := r.MultipartForm.File["files"]
		if len(headers) != 1 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Import validation failed.",
				httpresponse.NewFieldProblem("files", "Choose exactly one Kumbuka export ZIP."),
			)
			return
		}

		archive, err := readPortableArchiveUpload(headers[0])
		if err != nil {
			writePortableArchiveImportProblem(logger, w, err)
			return
		}

		imported, err := restorePortableArchive(
			r.Context(),
			archive,
			pageUseCases,
			mediaUseCases,
			groupUseCases,
			currentUser(r),
		)
		if err != nil {
			writePortableArchiveImportProblem(logger, w, err)
			return
		}

		http.Redirect(w, r, "/admin/import?result="+strconv.Itoa(imported), http.StatusSeeOther)
	}
}

// readPortableArchiveUpload reads and validates one uploaded Kumbuka archive.
func readPortableArchiveUpload(header *multipart.FileHeader) (portableArchiveContents, error) {
	if strings.ToLower(path.Ext(header.Filename)) != ".zip" {
		return portableArchiveContents{}, newRequestError(
			"files",
			"Kumbuka imports require a .zip export archive.",
			errors.New("portable archive is not a ZIP"),
		)
	}

	file, err := header.Open()
	if err != nil {
		return portableArchiveContents{}, err
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil {
		return portableArchiveContents{}, err
	}
	if len(data) > maxImportBytes {
		return portableArchiveContents{}, newRequestError(
			"files",
			"Kumbuka archive exceeds 100 MiB.",
			errors.New("portable archive exceeds 100 MiB"),
		)
	}

	return parsePortableArchive(data)
}

// parsePortableArchive validates a Kumbuka archive completely before any data is written.
func parsePortableArchive(data []byte) (portableArchiveContents, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return portableArchiveContents{}, portableArchiveRequestError("The Kumbuka ZIP archive is invalid.", err)
	}

	entries := make(map[string]*zip.File, len(reader.File))
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		name, err := validPortableArchivePath(entry.Name)
		if err != nil {
			return portableArchiveContents{}, err
		}
		if _, exists := entries[name]; exists {
			return portableArchiveContents{}, portableArchiveRequestError(
				"The Kumbuka archive contains duplicate paths.",
				fmt.Errorf("duplicate archive path %q", name),
			)
		}
		entries[name] = entry
	}

	manifestEntry, ok := entries[portable.ManifestPath]
	if !ok {
		return portableArchiveContents{}, portableArchiveRequestError(
			"The Kumbuka archive is missing manifest.json.",
			errors.New("portable archive manifest is missing"),
		)
	}

	remaining := int64(maxImportBytes)
	manifestData, err := readPortableZipFile(manifestEntry, &remaining)
	if err != nil {
		return portableArchiveContents{}, err
	}

	var manifest portable.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return portableArchiveContents{}, portableArchiveRequestError(
			"The Kumbuka archive manifest contains invalid JSON.",
			fmt.Errorf("decode portable manifest: %w", err),
		)
	}
	if manifest.Format != portable.Format {
		return portableArchiveContents{}, portableArchiveRequestError(
			"The ZIP is not a Kumbuka portable archive.",
			fmt.Errorf("unexpected portable archive format %q", manifest.Format),
		)
	}
	if manifest.Version != portable.Version {
		return portableArchiveContents{}, portableArchiveRequestError(
			fmt.Sprintf("Kumbuka archive version %d is not supported by this build.", manifest.Version),
			fmt.Errorf("unsupported portable archive version %d", manifest.Version),
		)
	}
	if len(manifest.Pages) == 0 {
		return portableArchiveContents{}, portableArchiveRequestError(
			"The Kumbuka archive contains no pages.",
			errors.New("portable archive contains no pages"),
		)
	}

	contents := portableArchiveContents{
		Manifest:  manifest,
		Pages:     make([]portableArchivePage, 0, len(manifest.Pages)),
		Resources: map[string][]byte{},
	}
	used := map[string]bool{portable.ManifestPath: true}

	for _, pageEntry := range manifest.Pages {
		pageData, err := readPortableArchivePage(entries, pageEntry, used, &remaining)
		if err != nil {
			return portableArchiveContents{}, err
		}
		contents.Pages = append(contents.Pages, pageData)
	}

	for _, resource := range manifest.Media {
		if err := readPortableArchiveResource(entries, resource, "media/", used, &remaining, contents.Resources); err != nil {
			return portableArchiveContents{}, err
		}
	}
	for _, resource := range manifest.Attachments {
		if err := readPortableArchiveResource(entries, resource, "attachments/", used, &remaining, contents.Resources); err != nil {
			return portableArchiveContents{}, err
		}
	}

	if len(used) != len(entries) {
		return portableArchiveContents{}, portableArchiveRequestError(
			"The Kumbuka archive contains files that are not listed in its manifest.",
			errors.New("portable archive contains unlisted files"),
		)
	}

	return contents, nil
}

// readPortableArchivePage validates and reads one page and its metadata sidecar.
func readPortableArchivePage(
	entries map[string]*zip.File,
	pageEntry portable.PageEntry,
	used map[string]bool,
	remaining *int64,
) (portableArchivePage, error) {
	slug := strings.TrimSpace(pageEntry.Slug)
	if !validPortableSlug(slug) {
		return portableArchivePage{}, portableArchiveRequestError(
			"The Kumbuka archive contains an invalid page path.",
			fmt.Errorf("invalid portable page slug %q", pageEntry.Slug),
		)
	}

	expectedMarkdown := path.Join("pages", slug+".md")
	expectedMetadata := path.Join("metadata", slug+".json")
	if pageEntry.Markdown != expectedMarkdown || pageEntry.Metadata != expectedMetadata {
		return portableArchivePage{}, portableArchiveRequestError(
			"The Kumbuka archive page inventory is inconsistent.",
			fmt.Errorf("page %q paths do not match canonical paths", slug),
		)
	}

	markdown, err := readPortableManifestFile(entries, pageEntry.Markdown, used, remaining)
	if err != nil {
		return portableArchivePage{}, err
	}
	metadataData, err := readPortableManifestFile(entries, pageEntry.Metadata, used, remaining)
	if err != nil {
		return portableArchivePage{}, err
	}

	var metadata portable.PageMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		return portableArchivePage{}, portableArchiveRequestError(
			"The Kumbuka archive contains invalid page metadata.",
			fmt.Errorf("decode metadata for %q: %w", slug, err),
		)
	}
	if metadata.Slug != slug || strings.TrimSpace(metadata.Title) == "" {
		return portableArchivePage{}, portableArchiveRequestError(
			"The Kumbuka archive contains inconsistent page metadata.",
			fmt.Errorf("metadata for %q has slug %q or empty title", slug, metadata.Slug),
		)
	}
	if !domain.ValidPageStatus(metadata.Status) || !domain.ValidReviewIntervalDays(metadata.ReviewIntervalDays) {
		return portableArchivePage{}, portableArchiveRequestError(
			"The Kumbuka archive contains invalid page workflow metadata.",
			fmt.Errorf("invalid workflow metadata for %q", slug),
		)
	}
	if err := validatePortableGroupNames(metadata); err != nil {
		return portableArchivePage{}, err
	}

	return portableArchivePage{
		Entry:    pageEntry,
		Metadata: metadata,
		Markdown: string(markdown),
	}, nil
}

// readPortableArchiveResource validates and reads one manifest resource entry.
func readPortableArchiveResource(
	entries map[string]*zip.File,
	resource portable.ResourceEntry,
	prefix string,
	used map[string]bool,
	remaining *int64,
	resources map[string][]byte,
) error {
	if !strings.HasPrefix(resource.Path, prefix) || path.Base(resource.Path) != resource.Filename || strings.TrimSpace(resource.Filename) == "" {
		return portableArchiveRequestError(
			"The Kumbuka archive contains an invalid resource inventory.",
			fmt.Errorf("invalid portable resource %q", resource.Path),
		)
	}
	if used[resource.Path] {
		return portableArchiveRequestError(
			"The Kumbuka archive contains duplicate resource entries.",
			fmt.Errorf("duplicate portable resource %q", resource.Path),
		)
	}

	data, err := readPortableManifestFile(entries, resource.Path, used, remaining)
	if err != nil {
		return err
	}
	resources[resource.Path] = data
	return nil
}

// readPortableManifestFile reads one manifest-listed path and marks it consumed.
func readPortableManifestFile(
	entries map[string]*zip.File,
	name string,
	used map[string]bool,
	remaining *int64,
) ([]byte, error) {
	if used[name] {
		return nil, portableArchiveRequestError(
			"The Kumbuka archive contains duplicate manifest references.",
			fmt.Errorf("duplicate manifest path %q", name),
		)
	}
	entry, ok := entries[name]
	if !ok {
		return nil, portableArchiveRequestError(
			"The Kumbuka archive is missing a file listed in its manifest.",
			fmt.Errorf("missing manifest path %q", name),
		)
	}

	data, err := readPortableZipFile(entry, remaining)
	if err != nil {
		return nil, err
	}
	used[name] = true
	return data, nil
}

// readPortableZipFile reads one ZIP entry without exceeding the shared uncompressed budget.
func readPortableZipFile(entry *zip.File, remaining *int64) ([]byte, error) {
	if entry.UncompressedSize64 > uint64(*remaining) {
		return nil, portableArchiveRequestError(
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
		return nil, portableArchiveRequestError(
			"Kumbuka archive contents exceed 100 MiB.",
			errors.New("portable archive contents exceed 100 MiB"),
		)
	}
	*remaining -= int64(len(data))
	return data, nil
}

// validPortableArchivePath normalizes one ZIP path and rejects traversal or platform-specific separators.
func validPortableArchivePath(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) {
		return "", portableArchiveRequestError(
			"The Kumbuka archive contains an invalid file path.",
			fmt.Errorf("invalid archive path %q", name),
		)
	}
	clean := path.Clean(name)
	if clean != name || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", portableArchiveRequestError(
			"The Kumbuka archive contains an unsafe file path.",
			fmt.Errorf("unsafe archive path %q", name),
		)
	}
	return clean, nil
}

// validPortableSlug reports whether slug is already a canonical relative page path.
func validPortableSlug(slug string) bool {
	return slug != "" && slug != "." && md.Slug(slug) == slug && !strings.HasPrefix(slug, "/") &&
		!strings.HasSuffix(slug, "/") && !strings.Contains(slug, "//") && path.Clean(slug) == slug
}

// validatePortableGroupNames rejects blank collaboration group references before any groups are created.
func validatePortableGroupNames(metadata portable.PageMetadata) error {
	for _, name := range append(slices.Clone(metadata.Groups), metadata.OwnerGroup) {
		if name != "" && strings.TrimSpace(name) == "" {
			return portableArchiveRequestError(
				"The Kumbuka archive contains an invalid collaboration group name.",
				errors.New("portable archive contains blank group name"),
			)
		}
	}
	return nil
}

// restorePortableArchive recreates resources, groups, and pages from a validated archive.
func restorePortableArchive(
	ctx context.Context,
	archive portableArchiveContents,
	pageUseCases portableArchivePageImportService,
	mediaUseCases portableArchiveImportMediaService,
	groupUseCases portableArchiveGroupService,
	actor domain.User,
) (int, error) {
	replacements := make(map[string]string, len(archive.Manifest.Media)+len(archive.Manifest.Attachments))

	for _, resource := range archive.Manifest.Media {
		image, err := mediaUseCases.UploadImage(ctx, resource.Filename, archive.Resources[resource.Path], actor)
		if err != nil {
			return 0, fmt.Errorf("restore image %q: %w", resource.Path, err)
		}
		replacements[resource.Path] = mediaURL(image.ID, image.Filename)
	}
	for _, resource := range archive.Manifest.Attachments {
		attachment, err := mediaUseCases.UploadAttachment(ctx, resource.Filename, archive.Resources[resource.Path], actor)
		if err != nil {
			return 0, fmt.Errorf("restore attachment %q: %w", resource.Path, err)
		}
		replacements[resource.Path] = attachmentItem(attachment).URL
	}

	groupIDs, err := ensurePortableGroups(ctx, groupUseCases, archive.Pages)
	if err != nil {
		return 0, err
	}

	pages := make([]service.PortableImportedPage, 0, len(archive.Pages))
	for _, pageData := range archive.Pages {
		markdown, err := restorePortableResourceReferences(pageData.Entry.Markdown, pageData.Markdown, replacements)
		if err != nil {
			return 0, err
		}

		groups := make([]int64, 0, len(pageData.Metadata.Groups))
		seenGroups := map[int64]bool{}
		for _, name := range pageData.Metadata.Groups {
			id := groupIDs[portableGroupKey(name)]
			if id > 0 && !seenGroups[id] {
				groups = append(groups, id)
				seenGroups[id] = true
			}
		}

		pages = append(pages, service.PortableImportedPage{
			Slug:               pageData.Metadata.Slug,
			Title:              pageData.Metadata.Title,
			Icon:               pageData.Metadata.Icon,
			Language:           pageData.Metadata.Language,
			Markdown:           markdown,
			Tags:               slices.Clone(pageData.Metadata.Tags),
			GroupIDs:           groups,
			Status:             pageData.Metadata.Status,
			OwnerGroupID:       groupIDs[portableGroupKey(pageData.Metadata.OwnerGroup)],
			ReviewIntervalDays: pageData.Metadata.ReviewIntervalDays,
			DeprecatedTarget:   pageData.Metadata.DeprecatedTarget,
			Properties:         clonePortableProperties(pageData.Metadata.Properties),
		})
	}

	return pageUseCases.ImportPortable(ctx, pages, actor)
}

// ensurePortableGroups resolves archive group names and creates missing groups in deterministic order.
func ensurePortableGroups(
	ctx context.Context,
	groupUseCases portableArchiveGroupService,
	pages []portableArchivePage,
) (map[string]int64, error) {
	groups, err := groupUseCases.Groups(ctx)
	if err != nil {
		return nil, err
	}

	ids := make(map[string]int64, len(groups))
	for _, group := range groups {
		ids[portableGroupKey(group.Name)] = group.ID
	}

	missingByKey := map[string]string{}
	for _, pageData := range pages {
		names := append(slices.Clone(pageData.Metadata.Groups), pageData.Metadata.OwnerGroup)
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			key := portableGroupKey(name)
			if ids[key] == 0 {
				missingByKey[key] = name
			}
		}
	}

	keys := make([]string, 0, len(missingByKey))
	for key := range missingByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group, err := groupUseCases.CreateGroup(ctx, missingByKey[key])
		if err != nil {
			return nil, fmt.Errorf("create imported group %q: %w", missingByKey[key], err)
		}
		ids[key] = group.ID
	}

	return ids, nil
}

// portableGroupKey returns the case-insensitive lookup key used for portable group mapping.
func portableGroupKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// restorePortableResourceReferences replaces archive-relative resource paths with target URLs.
func restorePortableResourceReferences(
	markdownPath, markdown string,
	replacements map[string]string,
) (string, error) {
	paths := make([]string, 0, len(replacements))
	for resourcePath := range replacements {
		paths = append(paths, resourcePath)
	}
	sort.Strings(paths)

	from := filepath.FromSlash(path.Dir(markdownPath))
	for _, resourcePath := range paths {
		relative, err := filepath.Rel(from, filepath.FromSlash(resourcePath))
		if err != nil {
			return "", err
		}
		markdown = strings.ReplaceAll(markdown, filepath.ToSlash(relative), replacements[resourcePath])
	}
	return markdown, nil
}

// clonePortableProperties copies page properties so service mutation cannot alias decoded metadata.
func clonePortableProperties(properties map[string]string) map[string]string {
	if len(properties) == 0 {
		return map[string]string{}
	}
	clone := make(map[string]string, len(properties))
	for key, value := range properties {
		clone[key] = value
	}
	return clone
}

// portableArchiveRequestError creates a safe user-facing archive validation error.
func portableArchiveRequestError(message string, cause error) error {
	return newRequestError("files", message, cause)
}

// writePortableArchiveImportProblem writes safe validation problems and logs unexpected restore failures.
func writePortableArchiveImportProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	if message, ok := userErrorMessage(err); ok {
		httpresponse.Problem(w,
			http.StatusBadRequest,
			"Import validation failed.",
			httpresponse.NewFieldProblem("files", message),
		)
		return
	}

	if tryWriteValidationProblem(w, err, "Archive import failed.") {
		return
	}

	httpresponse.InternalServerError(logger, w, err)
}
