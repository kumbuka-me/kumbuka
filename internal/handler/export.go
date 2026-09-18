package handler

import (
	"archive/zip"
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/pdf"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	xhtml "golang.org/x/net/html"
)

// ExportPageMarkdown exports one page as Markdown or as a ZIP when referenced images must be included.
func ExportPageMarkdown(
	catalogUseCases pageContentService,
	mediaUseCases imageContentService,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSpace(r.PathValue("slug"))
		if slug == "" {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Export validation failed.",
				httpresponse.NewFieldProblem("slug", "A page path is required."),
			)
			return
		}

		pageData, err := catalogUseCases.GetPage(r.Context(), slug)
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}
		if len(referencedImageIDs(pageData.Markdown)) == 0 {
			serveMarkdown(w, pageData)
			return
		}

		file, modTime, cleanup, err := createExportArchive(r.Context(), catalogUseCases, mediaUseCases, []string{slug})
		if err != nil {
			writeExportProblem(logger, w, err)
			return
		}

		defer cleanup()

		serveExportArchive(w, r, path.Base(slug)+".zip", file, modTime)
	}
}

// ExportPagePDF renders one page into a downloadable PDF using the configured PDF service.
func ExportPagePDF(
	catalogUseCases pageReportCatalogService,
	settingsUseCases settingsService,
	navigationUseCases navigationService,
	mediaUseCases imageContentService,
	accessUseCases pageAccessReader,
	renderer *md.Renderer,
	views *webview.Views,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		parameters, err := readExportParameters(w, r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid export request.")
			return
		}

		applicationSettings, err := settingsUseCases.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		pdfURL := effectivePDFURL(views.Runtime().PDFURL, applicationSettings.PDFURL)
		if pdfURL == "" {
			httpresponse.Problem(w,
				http.StatusServiceUnavailable,
				"PDF export is not configured. Configure a PDF service in Administration or set KUMBUKA__PDF_URL.",
			)
			return
		}

		slug := strings.TrimSpace(r.PathValue("slug"))
		if slug == "" {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Export validation failed.",
				httpresponse.NewFieldProblem("slug", "A page path is required."),
			)
			return
		}

		pageData, err := catalogUseCases.GetPage(r.Context(), slug)
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}

		pdfHeaders, err := settingsUseCases.PDFRequestHeaders(r.Context())
		if err != nil {
			logger.Error("load PDF request headers", "event", "pdf_headers_load_failed", "error", err)
			httpresponse.Problem(w, http.StatusServiceUnavailable, "PDF export is temporarily unavailable.")
			return
		}

		rendered, err := renderExportHTML(r.Context(), catalogUseCases,
			navigationUseCases, mediaUseCases, renderer, accessUseCases, currentUser(r), pageData, parameters)
		if err != nil {
			writeRenderedExportProblem(logger, w, err)
			return
		}

		language := cmp.Or(pageData.Language, applicationSettings.ContentLanguage)

		pdfFile, cleanup, err := pdf.Render(r.Context(), pdfURL, pageData.Title, language, rendered, pdfRequestHeaders(pdfHeaders))
		if err != nil {
			logger.Error("generate PDF", "event", "pdf_generation_failed", "slug", slug, "error", err)
			httpresponse.Problem(w,
				http.StatusBadGateway,
				"PDF could not be generated. Check the configured PDF service and try again.",
			)
			return
		}

		defer cleanup()

		filename := path.Base(slug) + ".pdf"

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
		http.ServeContent(w, r, filename, time.Now(), pdfFile)
	}
}

// ExportPages creates an archive containing selected Markdown pages and referenced images.
func ExportPages(
	catalogUseCases pageContentService,
	navigationUseCases navigationService,
	mediaUseCases imageContentService,
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

		if len(slugs) == 1 {
			pageData, err := catalogUseCases.GetPage(r.Context(), slugs[0])
			if err != nil {
				writePageProblem(logger, w, err)
				return
			}
			if len(referencedImageIDs(pageData.Markdown)) == 0 {
				serveMarkdown(w, pageData)
				return
			}
		}

		file, modTime, cleanup, err := createExportArchive(
			r.Context(),
			catalogUseCases,
			mediaUseCases,
			slugs,
		)
		if err != nil {
			writeExportProblem(logger, w, err)
			return
		}

		defer cleanup()

		filename := "kumbuka-export-" + time.Now().UTC().Format("20060102-150405") + ".zip"

		serveExportArchive(w, r, filename, file, modTime)
	}
}

// serveMarkdown writes one page as a plain Markdown attachment.
func serveMarkdown(w http.ResponseWriter, pageData domain.Page) {
	filename := path.Base(pageData.Slug) + ".md"

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))

	_, _ = io.WriteString(w, pageData.Markdown)
}

// createExportArchive builds a temporary ZIP file for selected pages and returns its cleanup function.
func createExportArchive(
	ctx context.Context,
	catalogUseCases pageContentService,
	mediaUseCases imageContentService,
	slugs []string,
) (archiveFile *os.File, modTime time.Time, cleanup func(), err error) {
	file, err := os.CreateTemp("", "kumbuka-export-*.zip")
	if err != nil {
		return nil, time.Time{}, nil, err
	}

	name := file.Name()
	cleanup = func() {
		_ = file.Close()
		_ = os.Remove(name)
	}

	if err := writeExportArchive(ctx, catalogUseCases, mediaUseCases, file, slugs); err != nil {
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

// serveExportArchive writes a prepared ZIP archive to the client.
func serveExportArchive(w http.ResponseWriter, r *http.Request, filename string, file *os.File, modTime time.Time) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
	http.ServeContent(w, r, filename, modTime, file)
}

// exportSlugs resolves either the explicit page selection or all page slugs.
func exportSlugs(r *http.Request, navigationUseCases navigationService) ([]string, error) {
	if r.FormValue("all") == "true" {
		pages, err := navigationUseCases.NavigationPages(r.Context())
		if err != nil {
			return nil, err
		}

		slugs := make([]string, 0, len(pages))

		for _, page := range pages {
			slugs = append(slugs, page.Slug)
		}

		return slugs, nil
	}

	seen := map[string]bool{}
	var slugs []string

	for _, slug := range r.Form["slug"] {
		slug = strings.TrimSpace(slug)
		if slug == "" || seen[slug] {
			continue
		}

		seen[slug] = true
		slugs = append(slugs, slug)
	}

	return slugs, nil
}

// writeExportArchive writes selected Markdown pages and each referenced image once.
func writeExportArchive(
	ctx context.Context,
	catalogUseCases pageContentService,
	mediaUseCases imageContentService,
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
		markdown, imageIDs, err := exportedMarkdown(
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

// exportedMarkdown rewrites stored media URLs to relative archive paths and returns referenced image identifiers.
func exportedMarkdown(
	ctx context.Context,
	mediaUseCases imageContentService,
	markdownPath, source string,
	imageCache map[int64]domain.ImageData,
) (content string, imageIDs []int64, err error) {
	seen := map[int64]bool{}
	var ids []int64
	var result strings.Builder
	for {
		reference, ok := nextMediaReference(source)
		if !ok {
			result.WriteString(source)
			break
		}
		result.WriteString(source[:reference.start])
		replacement, err := exportedImagePath(ctx, mediaUseCases, markdownPath, reference.id, imageCache)
		if err != nil {
			return "", nil, err
		}
		result.WriteString(replacement)
		if !seen[reference.id] {
			seen[reference.id] = true
			ids = append(ids, reference.id)
		}
		source = source[reference.end:]
	}
	return result.String(), ids, nil
}

// exportedImagePath resolves and caches an image and returns its archive-relative path.
func exportedImagePath(
	ctx context.Context,
	mediaUseCases imageContentService,
	markdownPath string,
	id int64,
	cache map[int64]domain.ImageData,
) (relativePath string, err error) {
	image, ok := cache[id]
	if !ok {
		var err error
		image, err = mediaUseCases.ImageContent(ctx, id)
		if err != nil {
			return "", &exportMediaError{cause: err}
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

// mediaReference identifies a stored image reference within Markdown source.
type mediaReference struct {
	// start and end store the corresponding values for media reference.
	start, end int
	// id identifies media reference.
	id int64
}

// nextMediaReference scans the same bare /media/ID/filename syntax used by exports.
func nextMediaReference(source string) (reference mediaReference, found bool) {
	for offset := 0; offset < len(source); {
		index := strings.Index(source[offset:], "/media/")
		if index < 0 {
			break
		}
		start := offset + index
		offset = start + len("/media/")
		end := offset
		for end < len(source) && source[end] >= '0' && source[end] <= '9' {
			end++
		}
		if end == offset {
			continue
		}
		if end >= len(source) || source[end] != '/' {
			continue
		}
		end++
		for end < len(source) && !strings.ContainsRune(" \t\n\r\f)\"'", rune(source[end])) {
			end++
		}
		id, ok := mediaImageID(source[start:end])
		if !ok {
			continue
		}
		return mediaReference{start: start, end: end, id: id}, true
	}
	return mediaReference{}, false
}

// mediaImageID validates a local stored-image path and extracts its numeric ID.
func mediaImageID(value string) (imageID int64, ok bool) {
	value, ok = strings.CutPrefix(value, "/media/")
	if !ok {
		return 0, false
	}
	rawID, filename, ok := strings.Cut(value, "/")
	if !ok {
		return 0, false
	}
	if rawID == "" || filename == "" {
		return 0, false
	}
	for _, digit := range rawID {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	return id, err == nil
}

// referencedImageIDs returns unique image identifiers referenced from Markdown source.
func referencedImageIDs(source string) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	for {
		reference, ok := nextMediaReference(source)
		if !ok {
			return ids
		}
		if !seen[reference.id] {
			seen[reference.id] = true
			ids = append(ids, reference.id)
		}
		source = source[reference.end:]
	}
}

// isRenderedMediaAttribute reports whether an HTML attribute can reference stored Kumbuka media.
func isRenderedMediaAttribute(element, attribute string) bool {
	switch element {
	case "img":
		return attribute == "src"
	case "a":
		return attribute == "href"
	default:
		return false
	}
}

// inlineRenderedMedia replaces authenticated media URLs with data URLs for standalone PDF rendering.
func inlineRenderedMedia(ctx context.Context, mediaUseCases imageContentService, rendered string) (html string, err error) {
	cache := map[int64]string{}
	var result strings.Builder
	tokens := xhtml.NewTokenizer(strings.NewReader(rendered))
	for {
		kind := tokens.Next()
		if kind == xhtml.ErrorToken {
			if err := tokens.Err(); err != io.EOF {
				return "", err
			}
			return result.String(), nil
		}
		raw := string(tokens.Raw())
		if kind != xhtml.StartTagToken && kind != xhtml.SelfClosingTagToken {
			result.WriteString(raw)
			continue
		}
		token := tokens.Token()
		changed := false
		for index := range token.Attr {
			attribute := &token.Attr[index]
			if !isRenderedMediaAttribute(token.Data, attribute.Key) {
				continue
			}

			location, err := url.Parse(attribute.Val)
			if err != nil {
				continue
			}
			if location.IsAbs() || location.Host != "" {
				continue
			}
			id, ok := mediaImageID(location.Path)
			if !ok {
				continue
			}
			dataURL, ok := cache[id]
			if !ok {
				image, err := mediaUseCases.ImageContent(ctx, id)
				if err != nil {
					return "", &exportMediaError{cause: err}
				}
				dataURL = "data:" + image.ContentType + ";base64," + base64.StdEncoding.EncodeToString(image.Data)
				cache[id] = dataURL
			}
			attribute.Val = dataURL
			changed = true
		}
		if changed {
			result.WriteString(token.String())
		} else {
			result.WriteString(raw)
		}
	}
}

// exportMediaError retains the origin of a media failure in a multi-resource export.
type exportMediaError struct {
	// cause stores the cause value used by export media error.
	cause error
}

// Error returns the media export failure message.
func (e *exportMediaError) Error() string { return fmt.Sprintf("export image: %v", e.cause) }

// Unwrap returns the underlying media lookup failure.
func (e *exportMediaError) Unwrap() error { return e.cause }

// writeExportProblem translates expected export failures into HTTP problems.
func writeExportProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	if _, media := errors.AsType[*exportMediaError](err); media {
		if errors.Is(err, domain.ErrNotFound) {
			httpresponse.Problem(w, http.StatusNotFound, "An image referenced by this export was not found.")
			return
		}
		httpresponse.InternalServerError(logger, w, err)
		return
	}
	writePageProblem(logger, w, err)
}
