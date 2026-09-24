package endpoint

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/application/portablearchive"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// ExportPortablePages creates a versioned archive containing pages, metadata, images, and attachments.
func ExportPortablePages(
	catalogUseCases pageContentService,
	navigationUseCases navigationService,
	mediaUseCases portablearchive.MediaExport,
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

		file, modTime, cleanup, err := createPortableExportArchive(
			r.Context(),
			catalogUseCases,
			mediaUseCases,
			slugs,
		)
		if err != nil {
			writePortableExportProblem(logger, w, err)
			return
		}
		defer cleanup()

		filename := "kumbuka-export-" + time.Now().UTC().Format("20060102-150405") + ".zip"
		serveExportArchive(w, r, filename, file, modTime)
	}
}

// createPortableExportArchive builds a temporary portable ZIP and returns its cleanup function.
func createPortableExportArchive(
	ctx context.Context,
	catalogUseCases pageContentService,
	mediaUseCases portablearchive.MediaExport,
	slugs []string,
) (archiveFile *os.File, modTime time.Time, cleanup func(), err error) {
	file, err := os.CreateTemp("", "kumbuka-portable-export-*.zip")
	if err != nil {
		return nil, time.Time{}, nil, err
	}

	name := file.Name()
	cleanup = func() {
		_ = file.Close()
		_ = os.Remove(name)
	}

	if err := portablearchive.WritePortable(ctx, catalogUseCases, mediaUseCases, file, slugs); err != nil {
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

// writePortableExportProblem translates expected portable resource failures into HTTP problems.
func writePortableExportProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	var resourceError *portablearchive.ResourceError
	if errors.As(err, &resourceError) {
		if errors.Is(err, domain.ErrNotFound) {
			message := "An image referenced by this export was not found."
			if resourceError.Kind == "attachments" {
				message = "An attachment referenced by this export was not found."
			}
			httpresponse.Problem(w, http.StatusNotFound, message)
			return
		}
		httpresponse.InternalServerError(logger, w, err)
		return
	}

	writePageProblem(logger, w, err)
}
