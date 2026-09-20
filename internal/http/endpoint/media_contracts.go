package endpoint

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// imageContentService supplies image bytes to export and sharing workflows.
type imageContentService interface {
	ImageContent(context.Context, int64) (domain.ImageData, error)
}

// imageListService lists and searches image metadata for administration.
type imageListService interface {
	Images(context.Context) ([]domain.Image, error)
	SearchImages(context.Context, string, int, int) ([]domain.Image, error)
}

// userImageService lists image metadata scoped to one uploader.
type userImageService interface {
	ImagesByUser(context.Context, int64) ([]domain.Image, error)
	SearchImagesByUser(context.Context, int64, string, int, int) ([]domain.Image, error)
}

// imageService owns image listing, upload, download, and deletion endpoints.
type imageService interface {
	ImageContent(context.Context, int64) (domain.ImageData, error)
	Images(context.Context) ([]domain.Image, error)
	ImagesByUser(context.Context, int64) ([]domain.Image, error)
	SearchImages(context.Context, string, int, int) ([]domain.Image, error)
	SearchImagesByUser(context.Context, int64, string, int, int) ([]domain.Image, error)
	UploadImage(context.Context, string, []byte, domain.User) (domain.Image, error)
	DeleteImage(context.Context, int64, domain.User) error
}

// attachmentAdminService supplies attachment inventory and administrator deletion.
type attachmentAdminService interface {
	Attachments(context.Context) ([]domain.Attachment, error)
	DeleteAttachment(context.Context, int64, domain.User) error
}

// attachmentService owns attachment listing, upload, download, and deletion endpoints.
type attachmentService interface {
	Attachments(context.Context) ([]domain.Attachment, error)
	AttachmentContent(context.Context, int64) (domain.AttachmentData, error)
	UploadAttachment(context.Context, string, []byte, domain.User) (domain.Attachment, error)
	DeleteAttachment(context.Context, int64, domain.User) error
}
