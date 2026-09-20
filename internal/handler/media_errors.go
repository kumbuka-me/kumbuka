package handler

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	appmedia "github.com/kumbuka-me/kumbuka/internal/application/media"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

type mediaKind int

const (
	imageMedia mediaKind = iota
	attachmentMedia
)

// mediaErrorText groups data used by media error text.
type mediaErrorText struct {
	// title is the title associated with media error text.
	title string
	// empty stores the empty value used by media error text.
	empty string
	// tooLarge stores the too large value used by media error text.
	tooLarge string
	// unsupported stores the unsupported value used by media error text.
	unsupported string
	// forbidden stores the forbidden value used by media error text.
	forbidden string
	// noun stores the noun value used by media error text.
	noun string
}

var mediaErrorTexts = map[mediaKind]mediaErrorText{
	imageMedia: {
		title:       "Image validation failed.",
		empty:       "The selected image is empty.",
		tooLarge:    "Images may be at most 10 MiB.",
		unsupported: "Only JPEG, PNG, GIF, and WebP images are supported.",
		forbidden:   "You can only delete images you uploaded.",
		noun:        "Image",
	},
	attachmentMedia: {
		title:       "Attachment validation failed.",
		empty:       "The selected file is empty.",
		tooLarge:    "Files may be at most 25 MiB.",
		unsupported: "Use PDF, TXT, Markdown, JSON, YAML, CSV, LOG, TOML, or ZIP files.",
		forbidden:   "You can only delete files you uploaded.",
		noun:        "Attachment",
	},
}

// writeMediaUploadProblem translates service upload failures into HTTP problems.
func writeMediaUploadProblem(
	logger *slog.Logger,
	w http.ResponseWriter,
	err error,
	kind mediaKind,
) {
	text := mediaErrorTexts[kind]

	switch {
	case errors.Is(err, appmedia.ErrEmptyFile):
		httpresponse.Problem(w,
			http.StatusBadRequest,
			text.title,
			httpresponse.NewFieldProblem("file", text.empty),
		)
	case errors.Is(err, appmedia.ErrFileTooLarge):
		httpresponse.Problem(w,
			http.StatusRequestEntityTooLarge,
			text.title,
			httpresponse.NewFieldProblem("file", text.tooLarge),
		)
	case errors.Is(err, appmedia.ErrUnsupportedFileType):
		httpresponse.Problem(w,
			http.StatusUnsupportedMediaType,
			text.title,
			httpresponse.NewFieldProblem("file", text.unsupported),
		)
	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}

// writeMediaDeleteProblem translates media policy failures into HTTP problems.
func writeMediaDeleteProblem(
	logger *slog.Logger,
	w http.ResponseWriter,
	err error,
	kind mediaKind,
) {
	text := mediaErrorTexts[kind]
	inUse, isInUse := errors.AsType[*appmedia.MediaInUseError](err)

	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w, http.StatusNotFound, text.noun+" not found.")
	case errors.Is(err, appmedia.ErrMediaForbidden):
		httpresponse.Problem(w, http.StatusForbidden, text.forbidden)
	case isInUse:
		httpresponse.Problem(w,
			http.StatusConflict,
			fmt.Sprintf("%s is still referenced %d time(s).", text.noun, inUse.References),
		)
	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}

// writeMediaReadProblem translates image and attachment lookup failures.
func writeMediaReadProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}
