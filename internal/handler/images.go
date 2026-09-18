package handler

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const (
	managedImagePageSize = 30
	maxImageAPILimit     = 100
)

// ListImages returns uploaded image metadata for editors and administrators.
func ListImages(mediaUseCases imageService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()
		query := strings.TrimSpace(values.Get("q"))
		limited := values.Has("limit") || values.Has("offset") || values.Has("scope") || query != ""

		if !limited {
			images, err := mediaUseCases.Images(r.Context())
			if err != nil {
				httpresponse.InternalServerError(logger, w, err)
				return
			}

			httpresponse.Respond(w, http.StatusOK, mediaItems(images))
			return
		}

		limit, offset, ok := imageListRange(w, values.Get("limit"), values.Get("offset"))
		if !ok {
			return
		}

		var (
			images []domain.Image
			err    error
		)

		switch values.Get("scope") {
		case "":
			images, err = mediaUseCases.SearchImages(r.Context(), query, limit, offset)
		case "mine":
			images, err = mediaUseCases.SearchImagesByUser(r.Context(), currentUser(r).ID, query, limit, offset)
		default:
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid image scope.")
			return
		}
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, mediaItems(images))
	}
}

// imageListRange calculates the visible range for a paginated image list.
func imageListRange(w http.ResponseWriter, rawLimit, rawOffset string) (limit, offset int, ok bool) {
	limit = managedImagePageSize
	if rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > maxImageAPILimit {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid image limit.")
			return 0, 0, false
		}
		limit = parsed
	}

	if rawOffset != "" {
		parsed, err := strconv.Atoi(rawOffset)
		if err != nil || parsed < 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid image offset.")
			return 0, 0, false
		}
		offset = parsed
	}

	return limit, offset, true
}

// UploadImage validates and stores an uploaded raster image.
func UploadImage(mediaUseCases imageService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)

		r.Body = http.MaxBytesReader(w, r.Body, service.MaxImageBytes+(1<<20))
		file, header, err := r.FormFile("file")
		if err != nil {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Image validation failed.",
				httpresponse.NewFieldProblem("file", "Choose an image file."),
			)
			return
		}

		defer file.Close() // nolint:errcheck

		data, err := io.ReadAll(io.LimitReader(file, service.MaxImageBytes+1))
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		image, err := mediaUseCases.UploadImage(r.Context(), header.Filename, data, user)
		if err != nil {
			writeMediaUploadProblem(logger, w, err, imageMedia)
			return
		}

		items := mediaItems([]domain.Image{image})

		httpresponse.Respond(w, http.StatusCreated, items[0])
	}
}

// ServeImage writes one stored image with immutable private caching headers.
func ServeImage(mediaUseCases imageService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		image, err := mediaUseCases.ImageContent(r.Context(), id)
		if err != nil {
			writeMediaReadProblem(logger, w, err)
			return
		}

		w.Header().Set("Content-Type", image.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(image.Data)))
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")

		_, _ = w.Write(image.Data)
	}
}

// DeleteImage removes an owned unused image or any image when requested by an administrator.
func DeleteImage(mediaUseCases imageService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid image identifier.")
			return
		}

		err = mediaUseCases.DeleteImage(r.Context(), id, user)
		if err != nil {
			writeMediaDeleteProblem(logger, w, err, imageMedia)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// managedImageItems converts stored images into administration view items.
func managedImageItems(images []domain.Image) (items []MediaItem, hasMore bool) {
	hasMore = len(images) > managedImagePageSize
	if hasMore {
		images = images[:managedImagePageSize]
	}

	return mediaItems(images), hasMore
}

// mediaItems converts store image metadata into browser-facing media items.
func mediaItems(images []domain.Image) []MediaItem {
	items := make([]MediaItem, 0, len(images))

	for _, image := range images {
		items = append(items, MediaItem{
			ID:          image.ID,
			Filename:    image.Filename,
			ContentType: image.ContentType,
			SizeBytes:   image.SizeBytes,
			UploadedBy:  image.UploadedBy,
			Uploader:    image.Uploader,
			CreatedAt:   image.CreatedAt,
			UsageCount:  image.UsageCount,
			URL:         mediaURL(image.ID, image.Filename),
		})
	}

	return items
}

// mediaURL builds the stable authenticated URL used in Markdown image references.
func mediaURL(id int64, filename string) string {
	return "/media/" + strconv.FormatInt(id, 10) + "/" + url.PathEscape(filename)
}
