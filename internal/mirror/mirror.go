// Package mirror exports the PostgreSQL-backed Kumbuka state as a deterministic,
// Git-friendly directory tree without changing the database source of truth.
package mirror

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// repository contains the database reads needed by the mirror exporter.
type repository interface {
	PageInventory(context.Context) ([]domain.Page, error)
	GetPage(context.Context, string) (domain.Page, error)
	Images(context.Context) ([]domain.Image, error)
	ImageContent(context.Context, int64) (domain.ImageData, error)
	Attachments(context.Context) ([]domain.Attachment, error)
	AttachmentContent(context.Context, int64) (domain.AttachmentData, error)
}

// pageMetadata is the portable sidecar written next to mirrored page content.
type pageMetadata struct {
	ID                 int64                 `json:"id"`
	Slug               string                `json:"slug"`
	Title              string                `json:"title"`
	Icon               string                `json:"icon,omitempty"`
	Language           string                `json:"language,omitempty"`
	CreatedBy          int64                 `json:"created_by"`
	UpdatedBy          int64                 `json:"updated_by"`
	Author             string                `json:"author,omitempty"`
	CreatedAt          string                `json:"created_at"`
	UpdatedAt          string                `json:"updated_at"`
	Tags               []string              `json:"tags,omitempty"`
	Groups             []domain.Group        `json:"groups,omitempty"`
	ViewCount          int64                 `json:"view_count"`
	Status             string                `json:"status"`
	OwnerGroupID       int64                 `json:"owner_group_id,omitempty"`
	OwnerGroup         string                `json:"owner_group,omitempty"`
	LastReviewedAt     string                `json:"last_reviewed_at,omitempty"`
	ReviewIntervalDays int                   `json:"review_interval_days,omitempty"`
	DeprecatedTarget   string                `json:"deprecated_target,omitempty"`
	Properties         []domain.PageProperty `json:"properties,omitempty"`
}

// manifest describes the stable mirror format and exported object inventory.
type manifest struct {
	Format      int      `json:"format"`
	Pages       []string `json:"pages"`
	Images      []int64  `json:"images,omitempty"`
	Attachments []int64  `json:"attachments,omitempty"`
}

// Export writes repository content atomically into outputDir.
func Export(ctx context.Context, repository repository, outputDir string) error {
	if err := validateOutputDir(outputDir); err != nil {
		return err
	}

	absolute, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	parent := filepath.Dir(absolute)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}

	temporary, err := os.MkdirTemp(parent, ".kumbuka-mirror-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary) // nolint:errcheck

	if err := exportInto(ctx, repository, temporary); err != nil {
		return err
	}
	if err := os.RemoveAll(absolute); err != nil {
		return err
	}
	return os.Rename(temporary, absolute)
}

// exportInto writes a complete mirror snapshot into an already-created directory.
func exportInto(ctx context.Context, repository repository, outputDir string) error {
	if err := createMirrorDirectories(outputDir); err != nil {
		return err
	}

	pages, err := exportPages(ctx, repository, outputDir)
	if err != nil {
		return err
	}

	images, err := exportImages(ctx, repository, outputDir)
	if err != nil {
		return err
	}

	attachments, err := exportAttachments(ctx, repository, outputDir)
	if err != nil {
		return err
	}

	return writeJSON(filepath.Join(outputDir, "manifest.json"), manifest{
		Format:      1,
		Pages:       pages,
		Images:      images,
		Attachments: attachments,
	})
}

// createMirrorDirectories creates the fixed directory layout used by a snapshot.
func createMirrorDirectories(outputDir string) error {
	for _, directory := range []string{"pages", "metadata", "media", "attachments"} {
		if err := os.MkdirAll(filepath.Join(outputDir, directory), 0o755); err != nil {
			return err
		}
	}

	return nil
}

// exportPages writes every active page and returns its stable path inventory.
func exportPages(ctx context.Context, repository repository, outputDir string) ([]string, error) {
	inventory, err := repository.PageInventory(ctx)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(inventory, func(left, right domain.Page) int {
		return strings.Compare(left.Slug, right.Slug)
	})

	pages := make([]string, 0, len(inventory))

	for _, summary := range inventory {
		page, err := repository.GetPage(ctx, summary.Slug)
		if err != nil {
			return nil, fmt.Errorf("read page %q: %w", summary.Slug, err)
		}

		if err := writePage(outputDir, page); err != nil {
			return nil, err
		}

		pages = append(pages, page.Slug)
	}

	return pages, nil
}

// exportImages writes every stored image and returns its stable identifier inventory.
func exportImages(ctx context.Context, repository repository, outputDir string) ([]int64, error) {
	images, err := repository.Images(ctx)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(images, func(left, right domain.Image) int {
		return compareID(left.ID, right.ID)
	})

	ids := make([]int64, 0, len(images))

	for _, image := range images {
		data, err := repository.ImageContent(ctx, image.ID)
		if err != nil {
			return nil, fmt.Errorf("read image %d: %w", image.ID, err)
		}

		if err := writeBinary(outputDir, "media", image.ID, data.Filename, data.Data); err != nil {
			return nil, err
		}

		ids = append(ids, image.ID)
	}

	return ids, nil
}

// exportAttachments writes every stored attachment and returns its stable identifier inventory.
func exportAttachments(ctx context.Context, repository repository, outputDir string) ([]int64, error) {
	attachments, err := repository.Attachments(ctx)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(attachments, func(left, right domain.Attachment) int {
		return compareID(left.ID, right.ID)
	})

	ids := make([]int64, 0, len(attachments))

	for _, attachment := range attachments {
		data, err := repository.AttachmentContent(ctx, attachment.ID)
		if err != nil {
			return nil, fmt.Errorf("read attachment %d: %w", attachment.ID, err)
		}

		if err := writeBinary(outputDir, "attachments", attachment.ID, data.Filename, data.Data); err != nil {
			return nil, err
		}

		ids = append(ids, attachment.ID)
	}

	return ids, nil
}

// writePage writes one page Markdown file and its metadata sidecar.
func writePage(outputDir string, page domain.Page) error {
	relative := cleanSlug(page.Slug)
	markdownPath := filepath.Join(outputDir, "pages", filepath.FromSlash(relative)+".md")
	if err := writeFile(markdownPath, []byte(page.Markdown)); err != nil {
		return fmt.Errorf("write page %q: %w", page.Slug, err)
	}

	metadata := pageMetadata{
		ID:                 page.ID,
		Slug:               page.Slug,
		Title:              page.Title,
		Icon:               page.Icon,
		Language:           page.Language,
		CreatedBy:          page.CreatedBy,
		UpdatedBy:          page.UpdatedBy,
		Author:             page.Author,
		CreatedAt:          page.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		UpdatedAt:          page.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
		Tags:               page.Tags,
		Groups:             page.Groups,
		ViewCount:          page.ViewCount,
		Status:             page.Status,
		OwnerGroupID:       page.OwnerGroupID,
		OwnerGroup:         page.OwnerGroup,
		ReviewIntervalDays: page.ReviewIntervalDays,
		DeprecatedTarget:   page.DeprecatedTarget,
		Properties:         page.Properties,
	}
	if page.LastReviewedAt != nil {
		metadata.LastReviewedAt = page.LastReviewedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")
	}
	return writeJSON(filepath.Join(outputDir, "metadata", filepath.FromSlash(relative)+".json"), metadata)
}

// writeBinary writes one uploaded object below its stable identifier.
func writeBinary(outputDir, kind string, id int64, filename string, data []byte) error {
	name := filepath.Base(filepath.FromSlash(filename))
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "file"
	}
	return writeFile(filepath.Join(outputDir, kind, fmt.Sprint(id), name), data)
}

// writeJSON writes deterministic indented JSON with a trailing newline.
func writeJSON(filename string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(filename, data)
}

// writeFile creates parent directories and writes one regular mirror file.
func writeFile(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o644)
}

// cleanSlug converts a Kumbuka page path into a safe mirror-relative path.
func cleanSlug(slug string) string {
	cleaned := strings.Trim(strings.TrimSpace(slug), "/")
	if cleaned == "" {
		return "index"
	}
	return cleaned
}

// compareID orders integer identifiers in ascending order.
func compareID(left, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

// validateOutputDir rejects empty or destructive mirror destinations.
func validateOutputDir(outputDir string) error {
	if strings.TrimSpace(outputDir) == "" {
		return errors.New("mirror output directory is required")
	}
	absolute, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	root := filepath.VolumeName(absolute) + string(filepath.Separator)
	if absolute == root {
		return errors.New("mirror output directory cannot be a filesystem root")
	}
	current, err := os.Getwd()
	if err != nil {
		return err
	}
	current, err = filepath.Abs(current)
	if err != nil {
		return err
	}
	if absolute == current {
		return errors.New("mirror output directory cannot be the current working directory")
	}
	return nil
}
