package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/service"
)

// AttachmentItem is browser-facing attachment metadata with a stable URL.
type AttachmentItem struct {
	domain.Attachment
	URL string `json:"url"`
}

// ListAttachments returns stored attachment metadata for editors.
func ListAttachments(mediaUseCases attachmentService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := mediaUseCases.Attachments(r.Context())
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		out := make([]AttachmentItem, 0, len(items))

		for _, item := range items {
			out = append(out, attachmentItem(item))
		}

		httpresponse.Respond(w, http.StatusOK, out)
	}
}

// UploadAttachment validates and stores an allowed documentation attachment.
func UploadAttachment(mediaUseCases attachmentService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		r.Body = http.MaxBytesReader(w, r.Body, service.MaxAttachmentBytes+(1<<20))
		file, header, err := r.FormFile("file")
		if err != nil {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Attachment validation failed.",
				httpresponse.NewFieldProblem("file", "Choose a file."),
			)
			return
		}

		defer file.Close() // nolint:errcheck

		data, err := io.ReadAll(io.LimitReader(file, service.MaxAttachmentBytes+1))
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		item, err := mediaUseCases.UploadAttachment(r.Context(), header.Filename, data, user)
		if err != nil {
			writeMediaUploadProblem(logger, w, err, attachmentMedia)
			return
		}

		httpresponse.Respond(w, http.StatusCreated, attachmentItem(item))
	}
}

// ServeAttachment downloads one stored attachment.
func ServeAttachment(mediaUseCases attachmentService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		item, err := mediaUseCases.AttachmentContent(r.Context(), id)
		if err != nil {
			writeMediaReadProblem(logger, w, err)
			return
		}

		w.Header().Set("Content-Type", item.ContentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(item.Data)))
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, item.Filename))
		w.Header().Set("X-Content-Type-Options", "nosniff")

		_, _ = w.Write(item.Data)
	}
}

// DeleteAttachment removes an owned unused attachment, or any attachment for administrators.
func DeleteAttachment(mediaUseCases attachmentService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid attachment identifier.")
			return
		}

		err = mediaUseCases.DeleteAttachment(r.Context(), id, user)
		if err != nil {
			writeMediaDeleteProblem(logger, w, err, attachmentMedia)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// attachmentItem adds a stable download URL to an attachment domain.
func attachmentItem(item domain.Attachment) AttachmentItem {
	return AttachmentItem{
		Attachment: item,
		URL: "/attachments/" + strconv.FormatInt(item.ID, 10) + "/" +
			url.PathEscape(item.Filename),
	}
}
