package media

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/filetype"
	"github.com/kumbuka-me/kumbuka/pkg/ascii"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const (
	// MaxImageBytes is the largest accepted image payload.
	MaxImageBytes = 10 << 20
	// MaxAttachmentBytes is the largest accepted attachment payload.
	MaxAttachmentBytes = 25 << 20
)

var (
	ErrEmptyFile           = errors.New("file is empty")
	ErrFileTooLarge        = errors.New("file is too large")
	ErrUnsupportedFileType = errors.New("unsupported file type")
	ErrMediaForbidden      = errors.New("media deletion is forbidden")
	ErrMediaInUse          = errors.New("media is still in use")
)

var (
	imageTypes = map[string][]string{
		"image/jpeg": {".jpg", ".jpeg"},
		"image/png":  {".png"},
		"image/gif":  {".gif"},
		"image/webp": {".webp"},
	}
	attachmentTypes = map[string]string{
		".pdf":  "application/pdf",
		".txt":  "text/plain; charset=utf-8",
		".md":   "text/markdown; charset=utf-8",
		".json": "application/json",
		".yaml": "application/yaml",
		".yml":  "application/yaml",
		".csv":  "text/csv; charset=utf-8",
		".log":  "text/plain; charset=utf-8",
		".toml": "application/toml",
		".zip":  "application/zip",
	}
)

// MediaInUseError reports how many page references prevent deletion.
type MediaInUseError struct {
	// References stores the references value used by media in use error.
	References int64
}

// mediaRepository is the persistence contract required by media use cases.
type mediaRepository interface {
	AttachmentInfo(context.Context, int64) (domain.Attachment, error)
	AttachmentContent(context.Context, int64) (domain.AttachmentData, error)
	Attachments(context.Context) ([]domain.Attachment, error)
	DeleteAttachment(context.Context, int64) error
	DeleteImage(context.Context, int64) error
	ImageContent(context.Context, int64) (domain.ImageData, error)
	ImageInfo(context.Context, int64) (domain.Image, error)
	Images(context.Context) ([]domain.Image, error)
	ImagesByUser(context.Context, int64) ([]domain.Image, error)
	SearchImages(context.Context, string, int, int) ([]domain.Image, error)
	SearchImagesByUser(context.Context, int64, string, int, int) ([]domain.Image, error)
	SaveAttachment(context.Context, string, string, []byte, int64) (domain.Attachment, error)
	SaveImage(context.Context, string, string, []byte, int64) (domain.Image, error)
}

// Error describes the number of references blocking media deletion.
func (e *MediaInUseError) Error() string {
	return fmt.Sprintf("media is still referenced %d time(s)", e.References)
}

// Unwrap exposes ErrMediaInUse for errors.Is checks.
func (e *MediaInUseError) Unwrap() error {
	return ErrMediaInUse
}

// Media coordinates upload validation and deletion authorization.
type Media struct {
	// repository provides the persistence operations required by media.
	repository mediaRepository
}

// NewMedia constructs the media application service.
func NewMedia(repository mediaRepository) *Media {
	return &Media{repository: repository}
}

// UploadImage validates, normalizes, and stores an image.
func (s *Media) UploadImage(ctx context.Context, filename string, data []byte, actor domain.User) (domain.Image, error) {
	if len(data) == 0 {
		return domain.Image{}, ErrEmptyFile
	}
	if len(data) > MaxImageBytes {
		return domain.Image{}, ErrFileTooLarge
	}

	contentType, supported := filetype.DetectImage(data)
	if !supported {
		return domain.Image{}, ErrUnsupportedFileType
	}

	return s.repository.SaveImage(ctx, SanitizeImageFilename(filename, contentType), contentType, data, actor.ID)
}

// DeleteImage deletes an image when the actor owns it and it is unused, or is an administrator.
func (s *Media) DeleteImage(ctx context.Context, id int64, actor domain.User) error {
	image, err := s.repository.ImageInfo(ctx, id)
	if err != nil {
		return err
	}
	if err := authorizeMediaDeletion(image.UploadedBy, image.UsageCount, actor); err != nil {
		return err
	}

	return s.repository.DeleteImage(ctx, id)
}

// UploadAttachment validates, normalizes, and stores a documentation attachment.
func (s *Media) UploadAttachment(ctx context.Context, filename string, data []byte, actor domain.User) (domain.Attachment, error) {
	if len(data) == 0 {
		return domain.Attachment{}, ErrEmptyFile
	}
	if len(data) > MaxAttachmentBytes {
		return domain.Attachment{}, ErrFileTooLarge
	}

	filename = sanitizeAttachmentFilename(filename)
	contentType, supported := attachmentTypes[strings.ToLower(filepath.Ext(filename))]
	if !supported {
		return domain.Attachment{}, ErrUnsupportedFileType
	}

	return s.repository.SaveAttachment(ctx, filename, contentType, data, actor.ID)
}

// DeleteAttachment applies the same ownership and usage policy as images.
func (s *Media) DeleteAttachment(ctx context.Context, id int64, actor domain.User) error {
	item, err := s.repository.AttachmentInfo(ctx, id)
	if err != nil {
		return err
	}
	if err := authorizeMediaDeletion(item.UploadedBy, item.UsageCount, actor); err != nil {
		return err
	}

	return s.repository.DeleteAttachment(ctx, id)
}

// authorizeMediaDeletion enforces shared ownership and usage rules for stored media.
func authorizeMediaDeletion(uploadedBy, usageCount int64, actor domain.User) error {
	if actor.IsAdministrator() {
		return nil
	}
	if uploadedBy != actor.ID {
		return ErrMediaForbidden
	}
	if usageCount > 0 {
		return &MediaInUseError{References: usageCount}
	}
	return nil
}

// SanitizeImageFilename produces a stable URL-friendly name with the detected extension.
func SanitizeImageFilename(filename, contentType string) string {
	name := sanitizeFilename(filename, "image")
	extensions := imageTypes[contentType]
	if len(extensions) == 0 {
		return strings.TrimSuffix(name, filepath.Ext(name))
	}

	extension := strings.ToLower(filepath.Ext(name))
	if extension == "" {
		return name + extensions[0]
	}
	if !slices.Contains(extensions, extension) {
		return strings.TrimSuffix(name, filepath.Ext(name)) + extensions[0]
	}

	return name
}

// sanitizeAttachmentFilename produces a safe attachment name with an extension.
func sanitizeAttachmentFilename(filename string) string {
	name := sanitizeFilename(filename, "attachment.txt")

	if filepath.Ext(name) == "" {
		name += ".txt"
	}

	return name
}

// sanitizeFilename reduces an uploaded basename to URL-safe characters.
func sanitizeFilename(filename, fallback string) string {
	name := strings.TrimSpace(filepath.Base(filename))
	var safe strings.Builder
	invalidRun := false
	for _, character := range name {
		if isSafeFilenameRune(character) {
			safe.WriteRune(character)
			invalidRun = false
		} else if !invalidRun {
			safe.WriteByte('-')
			invalidRun = true
		}
	}
	name = safe.String()
	name = strings.Trim(name, "-._")
	return cmp.Or(name, fallback)
}

// isSafeFilenameRune reports whether character can be preserved in a sanitized upload name.
func isSafeFilenameRune(character rune) bool {
	return ascii.IsAlphanumeric(character) || character == '.' || character == '_' || character == '-'
}

// Images returns all uploaded images with usage metadata.
func (s *Media) Images(ctx context.Context) ([]domain.Image, error) {
	return s.repository.Images(ctx)
}

// ImagesByUser returns images uploaded by one user.
func (s *Media) ImagesByUser(ctx context.Context, userID int64) ([]domain.Image, error) {
	return s.repository.ImagesByUser(ctx, userID)
}

// SearchImages returns a bounded set of images matching filename or uploader.
func (s *Media) SearchImages(ctx context.Context, query string, limit, offset int) ([]domain.Image, error) {
	return s.repository.SearchImages(ctx, query, limit, offset)
}

// SearchImagesByUser returns a bounded set of one user's images matching filename.
func (s *Media) SearchImagesByUser(
	ctx context.Context,
	userID int64,
	query string,
	limit, offset int,
) ([]domain.Image, error) {
	return s.repository.SearchImagesByUser(ctx, userID, query, limit, offset)
}

// ImageContent returns stored image bytes and metadata.
func (s *Media) ImageContent(ctx context.Context, id int64) (domain.ImageData, error) {
	return s.repository.ImageContent(ctx, id)
}

// Attachments returns all uploaded attachments with usage metadata.
func (s *Media) Attachments(ctx context.Context) ([]domain.Attachment, error) {
	return s.repository.Attachments(ctx)
}

// AttachmentContent returns stored attachment bytes and metadata.
func (s *Media) AttachmentContent(ctx context.Context, id int64) (domain.AttachmentData, error) {
	return s.repository.AttachmentContent(ctx, id)
}
