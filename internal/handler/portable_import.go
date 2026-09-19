package handler

import (
	"context"
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
	"github.com/kumbuka-me/kumbuka/internal/importer"
	"github.com/kumbuka-me/kumbuka/internal/portable"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
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

// ImportPagesWithPortableArchive extends the normal admin importer with Kumbuka portable archives.
func ImportPagesWithPortableArchive(
	pageUseCases portableArchivePageImportService,
	mediaUseCases portableArchiveImportMediaService,
	groupUseCases portableArchiveGroupService,
	logger *slog.Logger,
) http.HandlerFunc {
	legacy := ImportPages(pageUseCases, logger)

	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxImportRequestBytes)
		if err := r.ParseMultipartForm(importMultipartMemory); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Import is too large or invalid.")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll() // nolint:errcheck
		}

		headers := r.MultipartForm.File["files"]
		portableSelected := strings.TrimSpace(r.FormValue("format")) == portable.Format
		if !portableSelected {
			detected, err := detectPortableArchiveUpload(headers)
			if err != nil {
				writePortableArchiveImportProblem(logger, w, err)
				return
			}
			if !detected {
				legacy.ServeHTTP(w, r)
				return
			}
		}

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

// detectPortableArchiveUpload reports whether one uploaded ZIP declares the Kumbuka portable format.
func detectPortableArchiveUpload(headers []*multipart.FileHeader) (bool, error) {
	if len(headers) != 1 || strings.ToLower(path.Ext(headers[0].Filename)) != ".zip" {
		return false, nil
	}

	file, err := headers[0].Open()
	if err != nil {
		return false, err
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(file, importer.MaxBytes+1))
	if err != nil {
		return false, err
	}
	if int64(len(data)) > importer.MaxBytes {
		return false, newRequestError(
			"files",
			"Kumbuka archive exceeds 100 MiB.",
			errors.New("portable archive exceeds 100 MiB"),
		)
	}

	return portable.Detect(data, importer.MaxBytes), nil
}

// readPortableArchiveUpload reads and validates one uploaded Kumbuka archive.
func readPortableArchiveUpload(header *multipart.FileHeader) (portable.Archive, error) {
	if strings.ToLower(path.Ext(header.Filename)) != ".zip" {
		return portable.Archive{}, newRequestError(
			"files",
			"Kumbuka imports require a .zip export archive.",
			errors.New("portable archive is not a ZIP"),
		)
	}

	file, err := header.Open()
	if err != nil {
		return portable.Archive{}, err
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(file, importer.MaxBytes+1))
	if err != nil {
		return portable.Archive{}, err
	}
	if int64(len(data)) > importer.MaxBytes {
		return portable.Archive{}, newRequestError(
			"files",
			"Kumbuka archive exceeds 100 MiB.",
			errors.New("portable archive exceeds 100 MiB"),
		)
	}

	return portable.Parse(data, importer.MaxBytes)
}

// restorePortableArchive recreates resources, groups, and pages from a validated archive.
func restorePortableArchive(
	ctx context.Context,
	archive portable.Archive,
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
	pages []portable.Page,
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

// writePortableArchiveImportProblem writes safe validation problems and logs unexpected restore failures.
func writePortableArchiveImportProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	var archiveValidation *portable.ValidationError
	if errors.As(err, &archiveValidation) {
		httpresponse.Problem(w,
			http.StatusBadRequest,
			"Import validation failed.",
			httpresponse.NewFieldProblem("files", archiveValidation.Message),
		)
		return
	}

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
