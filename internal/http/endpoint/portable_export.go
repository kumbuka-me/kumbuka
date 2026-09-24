package endpoint

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/portable"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// portableArchiveExportMediaService exposes only the binary reads required by portable export.
type portableArchiveExportMediaService interface {
	ImageContent(context.Context, int64) (domain.ImageData, error)
	AttachmentContent(context.Context, int64) (domain.AttachmentData, error)
}

// portableResourceKind identifies the Kumbuka resource URL family represented by a reference.
type portableResourceKind string

const (
	portableMediaResource      portableResourceKind = "media"
	portableAttachmentResource portableResourceKind = "attachments"
)

// portableResourceReference identifies one stored resource reference in Markdown source.
type portableResourceReference struct {
	// Start is the byte offset where the resource URL begins.
	Start int
	// End is the byte offset immediately after the resource URL.
	End int
	// Kind identifies whether the reference points at an image or attachment.
	Kind portableResourceKind
	// ID is the source-instance resource identifier.
	ID int64
}

// portableExportResource caches one exported binary and its archive path.
type portableExportResource struct {
	// Path is the resource's portable archive path.
	Path string
	// Filename is the portable filename restored on import.
	Filename string
}

// portableExportState tracks resources already written while pages are exported.
type portableExportState struct {
	// Archive receives portable ZIP entries.
	Archive *zip.Writer
	// Media reads referenced resource payloads.
	Media portableArchiveExportMediaService
	// Images maps source image identifiers to already-written archive entries.
	Images map[int64]portableExportResource
	// Attachments maps source attachment identifiers to already-written archive entries.
	Attachments map[int64]portableExportResource
	// Manifest inventories every page and referenced resource written to Archive.
	Manifest portable.Manifest
}

// ExportPortablePages creates a versioned archive containing pages, metadata, images, and attachments.
func ExportPortablePages(
	catalogUseCases pageContentService,
	navigationUseCases navigationService,
	mediaUseCases portableArchiveExportMediaService,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid export request.")
			return
		}

		slugs, err := exportSlugs(r, navigationUseCases)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if len(slugs) == 0 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Export validation failed.",
				httpresponse.NewFieldProblem("slug", "Select at least one page to export."),
			)
			return
		}

		file, modTime, cleanup, err := createPortableExportArchive(
			r.Context(),
			catalogUseCases,
			mediaUseCases,
			slugs,
		)
		if err != nil {
			writePortableExportProblem(logger, w, err)
			return
		}
		defer cleanup()

		filename := "kumbuka-export-" + time.Now().UTC().Format("20060102-150405") + ".zip"
		serveExportArchive(w, r, filename, file, modTime)
	}
}

// createPortableExportArchive builds a temporary portable ZIP and returns its cleanup function.
func createPortableExportArchive(
	ctx context.Context,
	catalogUseCases pageContentService,
	mediaUseCases portableArchiveExportMediaService,
	slugs []string,
) (archiveFile *os.File, modTime time.Time, cleanup func(), err error) {
	file, err := os.CreateTemp("", "kumbuka-portable-export-*.zip")
	if err != nil {
		return nil, time.Time{}, nil, err
	}

	name := file.Name()
	cleanup = func() {
		_ = file.Close()
		_ = os.Remove(name)
	}

	if err := writePortableExportArchive(ctx, catalogUseCases, mediaUseCases, file, slugs); err != nil {
		cleanup()
		return nil, time.Time{}, nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, time.Time{}, nil, err
	}

	info, err := file.Stat()
	if err != nil {
		cleanup()
		return nil, time.Time{}, nil, err
	}

	return file, info.ModTime(), cleanup, nil
}

// writePortableExportArchive writes a deterministic portable archive to output.
func writePortableExportArchive(
	ctx context.Context,
	catalogUseCases pageContentService,
	mediaUseCases portableArchiveExportMediaService,
	output io.Writer,
	slugs []string,
) error {
	archive := zip.NewWriter(output)
	state := &portableExportState{
		Archive:     archive,
		Media:       mediaUseCases,
		Images:      map[int64]portableExportResource{},
		Attachments: map[int64]portableExportResource{},
		Manifest:    portable.NewManifest(),
	}

	slugs = slices.Clone(slugs)
	sort.Strings(slugs)

	for _, slug := range slugs {
		pageData, err := catalogUseCases.GetPage(ctx, slug)
		if err != nil {
			_ = archive.Close()
			return err
		}

		cleanSlug := strings.Trim(path.Clean("/"+pageData.Slug), "/")
		if cleanSlug == "" || cleanSlug == "." {
			_ = archive.Close()
			return fmt.Errorf("invalid page slug %q", pageData.Slug)
		}

		markdownPath := path.Join("pages", cleanSlug+".md")
		metadataPath := path.Join("metadata", cleanSlug+".json")
		markdown, err := state.rewriteMarkdown(ctx, markdownPath, pageData.Markdown)
		if err != nil {
			_ = archive.Close()
			return err
		}

		if err := writePortableZipBytes(archive, markdownPath, []byte(markdown)); err != nil {
			_ = archive.Close()
			return err
		}
		if err := writePortableZipJSON(archive, metadataPath, portableMetadata(pageData)); err != nil {
			_ = archive.Close()
			return err
		}

		state.Manifest.Pages = append(state.Manifest.Pages, portable.PageEntry{
			Slug:     cleanSlug,
			Markdown: markdownPath,
			Metadata: metadataPath,
		})
	}

	sort.Slice(state.Manifest.Media, func(i, j int) bool {
		return state.Manifest.Media[i].Path < state.Manifest.Media[j].Path
	})
	sort.Slice(state.Manifest.Attachments, func(i, j int) bool {
		return state.Manifest.Attachments[i].Path < state.Manifest.Attachments[j].Path
	})

	if err := writePortableZipJSON(archive, portable.ManifestPath, state.Manifest); err != nil {
		_ = archive.Close()
		return err
	}

	return archive.Close()
}

// portableMetadata converts one domain page into instance-portable metadata.
func portableMetadata(pageData domain.Page) portable.PageMetadata {
	tags := slices.Clone(pageData.Tags)
	sort.Strings(tags)

	groups := make([]string, 0, len(pageData.Groups))
	for _, group := range pageData.Groups {
		groups = append(groups, group.Name)
	}
	sort.Strings(groups)

	properties := make(map[string]string, len(pageData.Properties))
	for _, property := range pageData.Properties {
		properties[property.Key] = property.Value
	}

	return portable.PageMetadata{
		Slug:               pageData.Slug,
		Title:              pageData.Title,
		Icon:               pageData.Icon,
		Language:           pageData.Language,
		Tags:               tags,
		Groups:             groups,
		Status:             pageData.Status,
		OwnerGroup:         pageData.OwnerGroup,
		ReviewIntervalDays: pageData.ReviewIntervalDays,
		DeprecatedTarget:   pageData.DeprecatedTarget,
		Properties:         properties,
	}
}

// rewriteMarkdown replaces stored resource URLs with page-relative archive paths.
func (s *portableExportState) rewriteMarkdown(
	ctx context.Context,
	markdownPath, source string,
) (string, error) {
	return rewriteMarkdownResourceURLs(source, func(url string) (string, bool, error) {
		reference, ok := nextPortableResourceReference(url)
		if !ok || reference.Start != 0 || reference.End != len(url) {
			return "", false, nil
		}
		resource, err := s.exportResource(ctx, reference.Kind, reference.ID)
		if err != nil {
			return "", false, err
		}

		from := filepath.FromSlash(path.Dir(markdownPath))
		target := filepath.FromSlash(resource.Path)
		relative, err := filepath.Rel(from, target)
		if err != nil {
			return "", false, err
		}
		return filepath.ToSlash(relative), true, nil
	})
}

// exportResource writes one referenced resource once and returns its archive location.
func (s *portableExportState) exportResource(
	ctx context.Context,
	kind portableResourceKind,
	id int64,
) (portableExportResource, error) {
	switch kind {
	case portableMediaResource:
		if resource, ok := s.Images[id]; ok {
			return resource, nil
		}

		image, err := s.Media.ImageContent(ctx, id)
		if err != nil {
			return portableExportResource{}, &exportMediaError{cause: err}
		}
		resource := portableExportResource{
			Path:     path.Join(string(kind), strconv.FormatInt(id, 10), path.Base(image.Filename)),
			Filename: path.Base(image.Filename),
		}
		if err := writePortableZipBytes(s.Archive, resource.Path, image.Data); err != nil {
			return portableExportResource{}, err
		}
		s.Images[id] = resource
		s.Manifest.Media = append(s.Manifest.Media, portable.ResourceEntry{
			Path: resource.Path, Filename: resource.Filename,
		})
		return resource, nil

	case portableAttachmentResource:
		if resource, ok := s.Attachments[id]; ok {
			return resource, nil
		}

		attachment, err := s.Media.AttachmentContent(ctx, id)
		if err != nil {
			return portableExportResource{}, &exportAttachmentError{cause: err}
		}
		resource := portableExportResource{
			Path:     path.Join(string(kind), strconv.FormatInt(id, 10), path.Base(attachment.Filename)),
			Filename: path.Base(attachment.Filename),
		}
		if err := writePortableZipBytes(s.Archive, resource.Path, attachment.Data); err != nil {
			return portableExportResource{}, err
		}
		s.Attachments[id] = resource
		s.Manifest.Attachments = append(s.Manifest.Attachments, portable.ResourceEntry{
			Path: resource.Path, Filename: resource.Filename,
		})
		return resource, nil
	default:
		return portableExportResource{}, fmt.Errorf("unsupported portable resource kind %q", kind)
	}
}

// writePortableZipJSON encodes value as indented JSON with a trailing newline.
func writePortableZipJSON(archive *zip.Writer, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writePortableZipBytes(archive, name, data)
}

// writePortableZipBytes writes one file entry into a portable archive.
func writePortableZipBytes(archive *zip.Writer, name string, data []byte) error {
	entry, err := archive.Create(name)
	if err != nil {
		return err
	}
	_, err = entry.Write(data)
	return err
}

// nextPortableResourceReference returns the earliest stored image or attachment URL in source.
func nextPortableResourceReference(source string) (portableResourceReference, bool) {
	bestStart := len(source) + 1
	var best portableResourceReference

	for _, candidate := range []struct {
		// kind selects the media service used to load this resource.
		kind portableResourceKind
		// prefix identifies stored URLs belonging to that resource kind.
		prefix string
	}{
		{kind: portableMediaResource, prefix: "/media/"},
		{kind: portableAttachmentResource, prefix: "/attachments/"},
	} {
		reference, ok := findPortableResourceReference(source, candidate.prefix)
		if ok && reference.Start < bestStart {
			reference.Kind = candidate.kind
			bestStart = reference.Start
			best = reference
		}
	}

	return best, bestStart <= len(source)
}

// findPortableResourceReference skips malformed URLs to find the first valid stored resource of one kind.
func findPortableResourceReference(source, prefix string) (portableResourceReference, bool) {
	for offset := 0; offset < len(source); {
		relative := strings.Index(source[offset:], prefix)
		if relative < 0 {
			break
		}
		start := offset + relative
		digitsStart := start + len(prefix)
		offset = digitsStart

		digitsEnd := digitsStart
		for digitsEnd < len(source) && source[digitsEnd] >= '0' && source[digitsEnd] <= '9' {
			digitsEnd++
		}
		if !hasPortableResourceIDTerminator(source, digitsStart, digitsEnd) {
			continue
		}

		end := digitsEnd + 1
		for end < len(source) && !strings.ContainsRune(" \t\n\r\f)\"'", rune(source[end])) {
			end++
		}
		id, err := strconv.ParseInt(source[digitsStart:digitsEnd], 10, 64)
		if err != nil || !validPortableResourceReference(id, digitsEnd, end) {
			continue
		}

		return portableResourceReference{Start: start, End: end, ID: id}, true
	}
	return portableResourceReference{}, false
}

// hasPortableResourceIDTerminator reports whether a resource reference contains digits followed by a path separator.
func hasPortableResourceIDTerminator(source string, digitsStart, digitsEnd int) bool {
	return digitsEnd > digitsStart && digitsEnd < len(source) && source[digitsEnd] == '/'
}

// validPortableResourceReference reports whether a parsed resource ID is positive and has a non-empty filename suffix.
func validPortableResourceReference(id int64, digitsEnd, end int) bool {
	return id > 0 && end > digitsEnd+1
}

// exportAttachmentError retains the origin of an attachment failure in a portable export.
type exportAttachmentError struct {
	// cause is the underlying attachment lookup failure.
	cause error
}

// Error returns the attachment export failure message.
func (e *exportAttachmentError) Error() string { return fmt.Sprintf("export attachment: %v", e.cause) }

// Unwrap returns the underlying attachment lookup failure.
func (e *exportAttachmentError) Unwrap() error { return e.cause }

// writePortableExportProblem translates expected portable resource failures into HTTP problems.
func writePortableExportProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	var mediaError *exportMediaError
	if errors.As(err, &mediaError) {
		if errors.Is(err, domain.ErrNotFound) {
			httpresponse.Problem(w, http.StatusNotFound, "An image referenced by this export was not found.")
			return
		}
		httpresponse.InternalServerError(logger, w, err)
		return
	}

	var attachmentError *exportAttachmentError
	if errors.As(err, &attachmentError) {
		if errors.Is(err, domain.ErrNotFound) {
			httpresponse.Problem(w, http.StatusNotFound, "An attachment referenced by this export was not found.")
			return
		}
		httpresponse.InternalServerError(logger, w, err)
		return
	}

	writePageProblem(logger, w, err)
}
