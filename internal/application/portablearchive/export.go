package portablearchive

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/markdownurl"
	"github.com/kumbuka-me/kumbuka/internal/portable"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// CatalogExport loads page content to include in an archive.
type CatalogExport interface {
	GetPage(context.Context, string) (domain.Page, error)
}

// MediaExport loads binary resources referenced by archived pages.
type MediaExport interface {
	ImageContent(context.Context, int64) (domain.ImageData, error)
	AttachmentContent(context.Context, int64) (domain.AttachmentData, error)
}

// ResourceError identifies a missing or inaccessible resource during export.
type ResourceError struct {
	// Kind is either media or attachments.
	Kind string
	// Cause is the underlying resource lookup failure.
	Cause error
}

// Error reports the resource lookup failure.
func (e *ResourceError) Error() string { return fmt.Sprintf("export %s: %v", e.Kind, e.Cause) }

// Unwrap exposes the underlying resource error.
func (e *ResourceError) Unwrap() error { return e.Cause }

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
	Media MediaExport
	// Images maps source image identifiers to already-written archive entries.
	Images map[int64]portableExportResource
	// Attachments maps source attachment identifiers to already-written archive entries.
	Attachments map[int64]portableExportResource
	// Manifest inventories every page and referenced resource written to Archive.
	Manifest portable.Manifest
}

// WritePortable writes a deterministic portable archive to output.
func WritePortable(
	ctx context.Context,
	catalogUseCases CatalogExport,
	mediaUseCases MediaExport,
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

		if err := WriteZIPBytes(archive, markdownPath, []byte(markdown)); err != nil {
			_ = archive.Close()
			return err
		}
		if err := WriteZIPJSON(archive, metadataPath, portableMetadata(pageData)); err != nil {
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

	if err := WriteZIPJSON(archive, portable.ManifestPath, state.Manifest); err != nil {
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
		Status:             string(pageData.Status),
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
	return markdownurl.Rewrite(source, func(url string) (string, bool, error) {
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
			return portableExportResource{}, &ResourceError{Kind: "media", Cause: err}
		}
		resource := portableExportResource{
			Path:     path.Join(string(kind), strconv.FormatInt(id, 10), path.Base(image.Filename)),
			Filename: path.Base(image.Filename),
		}
		if err := WriteZIPBytes(s.Archive, resource.Path, image.Data); err != nil {
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
			return portableExportResource{}, &ResourceError{Kind: "attachments", Cause: err}
		}
		resource := portableExportResource{
			Path:     path.Join(string(kind), strconv.FormatInt(id, 10), path.Base(attachment.Filename)),
			Filename: path.Base(attachment.Filename),
		}
		if err := WriteZIPBytes(s.Archive, resource.Path, attachment.Data); err != nil {
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

// WriteZIPJSON encodes value as indented JSON with a trailing newline.
func WriteZIPJSON(archive *zip.Writer, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return WriteZIPBytes(archive, name, data)
}

// WriteZIPBytes writes one file entry into a portable archive.
func WriteZIPBytes(archive *zip.Writer, name string, data []byte) error {
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
