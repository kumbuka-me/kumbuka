package portablearchive

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/markdownurl"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// MediaImages loads image payloads referenced by classic Markdown exports.
type MediaImages interface {
	ImageContent(context.Context, int64) (domain.ImageData, error)
}

// WriteMarkdown writes selected Markdown pages and each referenced image once.
func WriteMarkdown(
	ctx context.Context,
	catalogUseCases CatalogExport,
	mediaUseCases MediaImages,
	output io.Writer,
	slugs []string,
) error {
	archive := zip.NewWriter(output)

	exportedImages := map[int64]bool{}
	imageCache := map[int64]domain.ImageData{}

	for _, slug := range slugs {
		pageData, err := catalogUseCases.GetPage(ctx, slug)
		if err != nil {
			return err
		}

		cleanSlug := strings.Trim(path.Clean("/"+pageData.Slug), "/")
		if cleanSlug == "" || cleanSlug == "." {
			return fmt.Errorf("invalid page slug %q", pageData.Slug)
		}

		markdownPath := path.Join("pages", cleanSlug+".md")
		markdown, imageIDs, err := ExportedMarkdown(
			ctx,
			mediaUseCases,
			markdownPath,
			pageData.Markdown,
			imageCache,
		)
		if err != nil {
			return err
		}

		entry, err := archive.Create(markdownPath)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(entry, markdown); err != nil {
			return err
		}

		for _, imageID := range imageIDs {
			if exportedImages[imageID] {
				continue
			}

			image := imageCache[imageID]
			imageEntry, err := archive.Create(
				path.Join("media", strconv.FormatInt(imageID, 10), path.Base(image.Filename)),
			)
			if err != nil {
				return err
			}
			if _, err := imageEntry.Write(image.Data); err != nil {
				return err
			}

			exportedImages[imageID] = true
		}
	}

	return archive.Close()
}

// ExportedMarkdown rewrites stored media URLs to relative archive paths and returns referenced image identifiers.
func ExportedMarkdown(
	ctx context.Context,
	mediaUseCases MediaImages,
	markdownPath, source string,
	imageCache map[int64]domain.ImageData,
) (content string, imageIDs []int64, err error) {
	seen := map[int64]bool{}
	var ids []int64
	result, err := markdownurl.Rewrite(source, func(url string) (string, bool, error) {
		reference, ok := exactPortableResourceReference(url, portableMediaResource)
		if !ok {
			return "", false, nil
		}
		replacement, err := exportedImagePath(ctx, mediaUseCases, markdownPath, reference.ID, imageCache)
		if err != nil {
			return "", false, err
		}
		if !seen[reference.ID] {
			seen[reference.ID] = true
			ids = append(ids, reference.ID)
		}
		return replacement, true, nil
	})
	if err != nil {
		return "", nil, err
	}
	return result, ids, nil
}

// exportedImagePath resolves and caches an image and returns its archive-relative path.
func exportedImagePath(
	ctx context.Context,
	mediaUseCases MediaImages,
	markdownPath string,
	id int64,
	cache map[int64]domain.ImageData,
) (relativePath string, err error) {
	image, ok := cache[id]
	if !ok {
		var err error
		image, err = mediaUseCases.ImageContent(ctx, id)
		if err != nil {
			return "", &ResourceError{Kind: "media", Cause: err}
		}
		cache[id] = image
	}
	target := filepath.FromSlash(path.Join("media", strconv.FormatInt(id, 10), path.Base(image.Filename)))
	from := filepath.FromSlash(path.Dir(markdownPath))
	relative, err := filepath.Rel(from, target)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(relative), nil
}

// MediaImageID validates a local stored-image path and extracts its numeric ID.
func MediaImageID(value string) (imageID int64, ok bool) {
	reference, ok := exactPortableResourceReference(value, portableMediaResource)
	if !ok {
		return 0, false
	}
	return reference.ID, true
}

// ReferencedImageIDs returns unique image identifiers referenced from Markdown source.
func ReferencedImageIDs(source string) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	for _, location := range markdownurl.Ranges(source) {
		url := source[location.Start:location.End]
		reference, ok := exactPortableResourceReference(url, portableMediaResource)
		if !ok {
			continue
		}
		if !seen[reference.ID] {
			seen[reference.ID] = true
			ids = append(ids, reference.ID)
		}
	}
	return ids
}
