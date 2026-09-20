package endpoint

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/pdf"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

const (
	maxExportParameters          = 128
	maxExportParameterBytes      = 8 << 10
	maxExportParameterTotalBytes = 64 << 10
)

// exportParametersRequest contains the request payload for export parameters request.
type exportParametersRequest struct {
	// Parameters maps keys to parameters values used by export parameters request.
	Parameters map[string]map[string]map[string]string `json:"parameters"`
}

// exportPreviewResponse contains the response payload for export preview response.
type exportPreviewResponse struct {
	// Document stores the document value used by export preview response.
	Document string `json:"document"`
}

// readExportParameters accepts request-local plugin export parameters only from POST bodies.
func readExportParameters(w http.ResponseWriter, r *http.Request) (map[string]map[string]map[string]string, error) {
	if r.Method != http.MethodPost {
		return nil, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
	request, err := decode[exportParametersRequest](w, r)
	if err != nil {
		return nil, err
	}
	if err := validateExportParameters(request.Parameters); err != nil {
		return nil, err
	}
	return request.Parameters, nil
}

// validateExportParameters bounds untrusted plugin export input before rendering.
func validateExportParameters(parameters map[string]map[string]map[string]string) error {
	count := 0
	total := 0
	for pluginID, modules := range parameters {
		if !validExportParameterKey(pluginID) {
			return domain.NewValidationError("parameters", "Plugin identifiers must be valid UTF-8 text of at most 128 bytes.")
		}
		for moduleID, values := range modules {
			if !validExportParameterKey(moduleID) {
				return domain.NewValidationError("parameters", "Plugin module identifiers must be valid UTF-8 text of at most 128 bytes.")
			}
			for key, value := range values {
				count++
				if count > maxExportParameters {
					return domain.NewValidationError("parameters", "Override at most 128 values per export.")
				}
				if !validExportParameterKey(key) {
					return domain.NewValidationError("parameters", "Use exact resource keys of at most 128 UTF-8 bytes.")
				}
				if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
					return domain.NewValidationError("parameters", "Temporary values must contain valid UTF-8 text without null characters.")
				}
				if len(value) > maxExportParameterBytes {
					return domain.NewValidationError("parameters", "Each temporary value must be at most 8 KiB.")
				}
				total += len(pluginID) + len(moduleID) + len(key) + len(value)
				if total > maxExportParameterTotalBytes {
					return domain.NewValidationError("parameters", "Temporary values must total at most 64 KiB.")
				}
			}
		}
	}
	return nil
}

// validExportParameterKey reports whether one nested export parameter key is safe and bounded.
func validExportParameterKey(value string) bool {
	return value != "" && len(value) <= 128 && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n{}")
}

// renderExportHTML renders the shared self-contained page body used by print preview and PDF export.
func renderExportHTML(
	ctx context.Context,
	catalog pageReportCatalogService,
	navigation navigationService,
	media imageContentService,
	renderer *md.Renderer,
	access pageAccessReader,
	user domain.User,
	page domain.Page,
	parameters map[string]map[string]map[string]string,
) (string, error) {
	securedCatalog := accessiblePageCatalog{catalog: catalog, access: access, user: user}
	pageNavigation, err := subpageNavigation(ctx, navigation, access, user, page.Slug)
	if err != nil {
		return "", err
	}
	rendered, err := renderer.RenderPageResolvedWithFunctions(
		page.Markdown,
		md.Slug,
		md.DefaultOptions(),
		md.Functions{
			Context:          ctx,
			PluginUsage:      page.PluginUsage,
			Capabilities:     plugincap.Capabilities(securedCatalog, pageNavigation, renderer.IconCatalog()),
			ExportParameters: parameters,
		},
	)
	if err != nil {
		return "", err
	}
	return inlineRenderedMedia(ctx, media, rendered.HTML)
}

// PreviewPageExport returns a self-contained script-free print document without calling the PDF service.
func PreviewPageExport(
	catalog pageReportCatalogService,
	settings settingsService,
	navigation navigationService,
	media imageContentService,
	access pageAccessReader,
	renderer *md.Renderer,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		parameters, err := readExportParameters(w, r)
		if err != nil {
			if tryWriteValidationProblem(w, err, "Export validation failed.") {
				return
			}
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid export request.")
			return
		}
		slug := strings.TrimSpace(r.PathValue("slug"))
		if slug == "" {
			httpresponse.Problem(w, http.StatusBadRequest, "A page path is required.")
			return
		}
		page, err := catalog.GetPage(r.Context(), slug)
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}
		application, err := settings.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		rendered, err := renderExportHTML(r.Context(), catalog, navigation, media, renderer, access, currentUser(r), page, parameters)
		if err != nil {
			writeRenderedExportProblem(logger, w, err)
			return
		}
		language := cmp.Or(page.Language, application.ContentLanguage)
		httpresponse.Respond(w, http.StatusOK, exportPreviewResponse{Document: pdf.Document(page.Title, language, rendered)})
	}
}

// writeRenderedExportProblem translates expected rendered-export failures and hides infrastructure errors.
func writeRenderedExportProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	if tryWriteValidationProblem(w, err, "Export validation failed.") {
		return
	}
	var parameterError *plugin.ParameterError
	if errors.As(err, &parameterError) {
		httpresponse.Problem(w, http.StatusUnprocessableEntity, "Export validation failed.", httpresponse.NewFieldProblem("parameters", parameterError.Error()))
		return
	}
	if _, ok := errors.AsType[*exportMediaError](err); ok {
		writeExportProblem(logger, w, err)
		return
	}
	httpresponse.InternalServerError(logger, w, err)
}
