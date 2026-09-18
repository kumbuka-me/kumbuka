package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/service"
	xhtml "golang.org/x/net/html"
)

const maxImportBytes = 100 << 20

// importCandidate groups data used by import candidate.
type importCandidate struct {
	// Slug is the normalized page path associated with import candidate.
	Slug string
	// Title is the title associated with import candidate.
	Title string
	// Markdown stores the markdown value used by import candidate.
	Markdown string
	// Source records the source associated with import candidate.
	Source string
}

type importFormat string

const (
	markdownImport   importFormat = "markdown"
	wikiJSImport     importFormat = "wikijs"
	confluenceImport importFormat = "confluence"
)

// parseImportFormat validates an explicitly selected import source format.
func parseImportFormat(value string) (importFormat, error) {
	format := importFormat(strings.TrimSpace(value))
	switch format {
	case markdownImport, wikiJSImport, confluenceImport:
		return format, nil
	default:
		return "", newRequestError(
			"format",
			"Choose a source format.",
			fmt.Errorf("invalid import source format %q", value),
		)
	}
}

// AdminImport renders the import workspace.
func AdminImport(viewDataUseCases viewDataService, views *Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Import", "import")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.Query = r.URL.Query().Get("result")

		render(views, w, "admin_import", data)
	}
}

// ImportPages imports files using the explicitly selected source format.
func ImportPages(pageUseCases pageImportService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
		if err := r.ParseMultipartForm(maxImportBytes); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Import is too large or invalid.")
			return
		}

		format, err := parseImportFormat(r.FormValue("format"))
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Import validation failed.", "format", err) {
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}

		var candidates []importCandidate
		remaining := int64(maxImportBytes)

		for _, header := range r.MultipartForm.File["files"] {
			items, err := importCandidatesFromFile(header, format, &remaining)
			if err != nil {
				message, ok := userErrorMessage(err)
				if !ok {
					httpresponse.InternalServerError(logger, w, err)
					return
				}

				httpresponse.Problem(w,
					http.StatusBadRequest,
					"Import validation failed.",
					httpresponse.NewFieldProblem("files", header.Filename+": "+message),
				)
				return
			}

			candidates = append(candidates, items...)
		}

		if len(candidates) == 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Choose at least one supported import file.")
			return
		}

		importPages := make([]service.ImportedPage, 0, len(candidates))

		for _, candidate := range candidates {
			importPages = append(importPages, service.ImportedPage{
				Slug:     candidate.Slug,
				Title:    candidate.Title,
				Markdown: candidate.Markdown,
				Source:   candidate.Source,
			})
		}

		imported, err := pageUseCases.Import(r.Context(), importPages, string(format), user)
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}

		http.Redirect(w, r, "/admin/import?result="+strconv.Itoa(imported), http.StatusSeeOther)
	}
}

// importCandidatesFromFile extracts import candidates from one uploaded file.
func importCandidatesFromFile(header *multipart.FileHeader, format importFormat, remaining *int64) ([]importCandidate, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > maxImportBytes {
		return nil, newRequestError("files", "File exceeds 100 MiB.", errors.New("file exceeds 100 MiB"))
	}

	name := strings.TrimPrefix(strings.ReplaceAll(header.Filename, "\\", "/"), "./")
	ext := strings.ToLower(path.Ext(name))

	if ext != ".zip" {
		if int64(len(data)) > *remaining {
			return nil, newRequestError("files", "Import contents exceed 100 MiB.", errors.New("import contents exceed 100 MiB"))
		}
		*remaining -= int64(len(data))
	}

	switch format {
	case markdownImport:
		if ext != ".md" && ext != ".markdown" {
			if ext == ".zip" {
				return importZIP(data, format, remaining)
			}
			return nil, newRequestError("files", "Markdown imports require .md, .markdown, or .zip files.", errors.New("markdown import has unsupported file type"))
		}

		title, err := markdownTitle(string(data))
		if err != nil {
			return nil, err
		}

		slug := strings.TrimSuffix(name, ext)

		return []importCandidate{{Slug: slug, Title: title, Markdown: string(data), Source: "Markdown"}}, nil
	case wikiJSImport:
		if ext == ".zip" {
			return importZIP(data, format, remaining)
		}
		if ext != ".json" {
			return nil, newRequestError("files", "Imports from Wiki.js require .json or .zip files.", errors.New("validate Wiki.js import: unsupported file type"))
		}

		return importWikiJSON(data)
	case confluenceImport:
		if ext == ".zip" {
			return importZIP(data, format, remaining)
		}
		if ext != ".html" && ext != ".htm" {
			return nil, newRequestError("files", "Imports from Confluence require .html, .htm, or .zip files.", errors.New("validate Confluence import: unsupported file type"))
		}

		markdown, err := htmlToMarkdown(data)
		if err != nil {
			return nil, err
		}

		title, err := markdownTitle(markdown)
		if err != nil {
			return nil, err
		}

		slug := strings.TrimSuffix(name, ext)

		return []importCandidate{{
			Slug: slug, Title: title, Markdown: markdown, Source: "Confluence HTML",
		}}, nil
	default:
		return nil, fmt.Errorf("unsupported source format %q", format)
	}
}

// importZIP extracts supported import candidates from a ZIP archive.
func importZIP(data []byte, format importFormat, remaining *int64) ([]importCandidate, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, newRequestError(
			"files",
			"The ZIP archive is invalid.",
			fmt.Errorf("open ZIP archive: %w", err),
		)
	}

	var result []importCandidate

	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}

		name := strings.TrimPrefix(strings.ReplaceAll(entry.Name, "\\", "/"), "./")
		ext := strings.ToLower(path.Ext(name))
		if !supportedImportExtension(format, ext) {
			continue
		}
		if entry.UncompressedSize64 > uint64(*remaining) {
			return nil, newRequestError("files", "Archive contents exceed 100 MiB.", errors.New("archive contents exceed 100 MiB"))
		}

		file, err := entry.Open()
		if err != nil {
			return nil, err
		}

		content, err := readImportArchiveEntry(file, *remaining)
		if err != nil {
			return nil, wrapImportEntryError(name, err)
		}

		*remaining -= int64(len(content))

		switch format {
		case markdownImport:
			title, err := markdownTitle(string(content))
			if err != nil {
				return nil, wrapImportEntryError(name, err)
			}

			slug := strings.TrimSuffix(name, ext)
			result = append(result, importCandidate{
				Slug:     slug,
				Title:    title,
				Markdown: string(content),
				Source:   "ZIP/Markdown",
			})
		case wikiJSImport:
			items, err := importWikiJSON(content)
			if err != nil {
				return nil, wrapImportEntryError(name, err)
			}

			result = append(result, items...)
		case confluenceImport:
			markdown, err := htmlToMarkdown(content)
			if err != nil {
				return nil, wrapImportEntryError(name, err)
			}

			title, err := markdownTitle(markdown)
			if err != nil {
				return nil, wrapImportEntryError(name, err)
			}

			slug := strings.TrimSuffix(name, ext)
			result = append(result, importCandidate{
				Slug:     slug,
				Title:    title,
				Markdown: markdown,
				Source:   "Confluence ZIP",
			})
		}
	}

	return result, nil
}

// wrapImportEntryError preserves an archive entry name in safe and diagnostic import errors.
func wrapImportEntryError(name string, err error) error {
	diagnostic := fmt.Errorf("%s: %w", name, err)

	message, ok := userErrorMessage(err)
	if !ok {
		return diagnostic
	}

	return newRequestError("files", name+": "+message, diagnostic)
}

// supportedImportExtension reports whether a file extension belongs to a selected import format.
func supportedImportExtension(format importFormat, extension string) bool {
	switch format {
	case markdownImport:
		return extension == ".md" || extension == ".markdown"
	case wikiJSImport:
		return extension == ".json"
	case confluenceImport:
		return extension == ".html" || extension == ".htm"
	default:
		return false
	}
}

// readImportArchiveEntry reads and closes one entry without exceeding the archive budget.
func readImportArchiveEntry(file io.ReadCloser, remaining int64) ([]byte, error) {
	content, readErr := io.ReadAll(io.LimitReader(file, remaining+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(content)) > remaining {
		return nil, newRequestError("files", "Archive contents exceed 100 MiB.", errors.New("archive contents exceed 100 MiB"))
	}

	return content, nil
}

// importWikiJSON decodes pages from a Wiki.js JSON export.
func importWikiJSON(data []byte) ([]importCandidate, error) {
	var pages []struct {
		Path    string `json:"path"`
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(data, &pages); err != nil {
		return nil, newRequestError(
			"files",
			"The Wiki.js export contains invalid JSON.",
			fmt.Errorf("decode Wiki.js import: %w", err),
		)
	}
	if len(pages) == 0 {
		return nil, newRequestError("files", "The Wiki.js export must contain at least one page.", errors.New("validate Wiki.js export: no pages"))
	}

	result := make([]importCandidate, 0, len(pages))

	for index, page := range pages {
		page.Path = strings.TrimSpace(page.Path)
		page.Title = strings.TrimSpace(page.Title)
		if page.Path == "" {
			return nil, newRequestError("files", fmt.Sprintf("Page %d from Wiki.js has no path.", index+1), fmt.Errorf("validate Wiki.js page %d: missing path", index+1))
		}
		if page.Title == "" {
			return nil, newRequestError("files", fmt.Sprintf("Page %d from Wiki.js has no title.", index+1), fmt.Errorf("validate Wiki.js page %d: missing title", index+1))
		}
		if page.Content == "" {
			return nil, newRequestError("files", fmt.Sprintf("Page %d from Wiki.js has no content.", index+1), fmt.Errorf("validate Wiki.js page %d: missing content", index+1))
		}

		result = append(result, importCandidate{
			Slug: page.Path, Title: page.Title, Markdown: page.Content, Source: "Wiki.js JSON",
		})
	}

	return result, nil
}

// markdownTitle returns the first level-one heading in a Markdown document.
func markdownTitle(markdown string) (title string, err error) {
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
	return "", newRequestError("files", "Document requires a level-one Markdown heading for its title.", errors.New("document requires a level-one Markdown heading for its title"))
}

// htmlMarkdownWriter converts the supported Confluence HTML subset into Markdown.
type htmlMarkdownWriter struct {
	// output stores the output value used by html markdown writer.
	output strings.Builder
}

// htmlToMarkdown converts the supported Confluence HTML subset to Markdown.
func htmlToMarkdown(data []byte) (markdown string, err error) {
	root, err := xhtml.Parse(bytes.NewReader(data))
	if err != nil {
		return "", newRequestError(
			"files",
			"The Confluence HTML could not be parsed.",
			fmt.Errorf("parse Confluence HTML: %w", err),
		)
	}

	writer := htmlMarkdownWriter{}
	writer.writeNode(root, 0)

	return normalizeImportedMarkdown(writer.output.String()), nil
}

// writeNode appends one supported HTML node and recursively processes its children.
func (w *htmlMarkdownWriter) writeNode(node *xhtml.Node, listDepth int) {
	if node.Type == xhtml.TextNode {
		w.writeText(node)
		return
	}
	if node.Type != xhtml.ElementNode && node.Type != xhtml.DocumentNode {
		return
	}

	tag := strings.ToLower(node.Data)

	switch tag {
	case "script", "style", "nav":
		return
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(tag[1] - '0')
		w.output.WriteString("\n\n" + strings.Repeat("#", level) + " ")
	case "p", "div", "section", "article":
		w.output.WriteString("\n\n")
	case "br":
		w.output.WriteByte('\n')
	case "strong", "b":
		w.output.WriteString("**")
	case "em", "i":
		w.output.WriteString("*")
	case "code":
		if node.Parent == nil || strings.ToLower(node.Parent.Data) != "pre" {
			w.output.WriteString("`")
		}
	case "pre":
		w.output.WriteString("\n\n```\n")
	case "li":
		w.output.WriteString("\n" + strings.Repeat("  ", listDepth) + "- ")
	case "ul", "ol":
		listDepth++
	case "a":
		w.output.WriteString("[")
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		w.writeNode(child, listDepth)
	}

	switch tag {
	case "strong", "b":
		w.output.WriteString("**")
	case "em", "i":
		w.output.WriteString("*")
	case "code":
		if node.Parent == nil || strings.ToLower(node.Parent.Data) != "pre" {
			w.output.WriteString("`")
		}
	case "pre":
		w.output.WriteString("\n```\n")
	case "a":
		w.writeAnchorTarget(node)
	case "p", "div", "section", "article", "h1", "h2", "h3", "h4", "h5", "h6":
		w.output.WriteString("\n")
	}
}

// writeText normalizes HTML text-node whitespace while preserving word boundaries.
func (w *htmlMarkdownWriter) writeText(node *xhtml.Node) {
	text := strings.Join(strings.Fields(node.Data), " ")
	if text == "" {
		return
	}

	leadingSpace := strings.TrimLeft(node.Data, " \t\r\n") != node.Data
	trailingSpace := strings.TrimRight(node.Data, " \t\r\n") != node.Data

	if leadingSpace && w.output.Len() > 0 {
		current := w.output.String()
		last := current[len(current)-1]
		if last != ' ' && last != '\n' {
			w.output.WriteByte(' ')
		}
	}

	w.output.WriteString(text)

	if trailingSpace {
		w.output.WriteByte(' ')
	}
}

// writeAnchorTarget closes a Markdown link using the HTML anchor's href attribute.
func (w *htmlMarkdownWriter) writeAnchorTarget(node *xhtml.Node) {
	href := ""

	for _, attr := range node.Attr {
		if attr.Key == "href" {
			href = attr.Val
			break
		}
	}

	w.output.WriteString("](" + href + ")")
}

// normalizeImportedMarkdown collapses repeated blank lines and trims trailing whitespace.
func normalizeImportedMarkdown(source string) string {
	lines := strings.Split(source, "\n")
	cleaned := make([]string, 0, len(lines))
	blank := false

	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			if blank {
				continue
			}

			blank = true
			cleaned = append(cleaned, "")
			continue
		}

		blank = false
		cleaned = append(cleaned, line)
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n")) + "\n"
}
