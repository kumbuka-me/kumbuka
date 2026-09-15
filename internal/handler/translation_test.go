package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestExtractedTranslatorsPreserveResponsesAndLogOnlyInternalFailures(t *testing.T) {
	t.Parallel()
	validation := domain.NewValidationError("name", "Required.")

	t.Run("media", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writeMediaReadProblem(logger, response, fmt.Errorf("wrapped: %w", domain.ErrNotFound))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "Not found.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writeMediaReadProblem(logger, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("permalink", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writePermalinkProblem(logger, response, fmt.Errorf("wrapped: %w", domain.ErrNotFound))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "Not found.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writePermalinkProblem(logger, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("token owner", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writeTokenCreateProblem(logger, response, fmt.Errorf("wrapped: %w", domain.ErrNotFound))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "User not found.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writeTokenCreateProblem(logger, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("token validation", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writeTokenCreateProblem(logger, response, fmt.Errorf("wrapped: %w", validation))
		assert.Equal(t, 422, response.Code)
		assert.Contains(t, response.Body.String(), "Required.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writeTokenCreateProblem(logger, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("token delete", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writeTokenDeleteProblem(logger, response, fmt.Errorf("wrapped: %w", domain.ErrNotFound))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "Token not found.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writeTokenDeleteProblem(logger, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("password", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writePasswordChangeProblem(logger, response, fmt.Errorf("wrapped: %w", auth.ErrInvalidCredentials))
		assert.Equal(t, 401, response.Code)
		assert.Contains(t, response.Body.String(), "The current password is incorrect.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writePasswordChangeProblem(logger, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("preferences", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writePreferencesProblem(logger, response, fmt.Errorf("wrapped: %w", validation))
		assert.Equal(t, 422, response.Code)
		assert.Contains(t, response.Body.String(), "Required.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writePreferencesProblem(logger, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("share", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writePublicShareError(logger, response, fmt.Errorf("wrapped: %w", domain.ErrNotFound))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "Share link not found or no longer available.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writePublicShareError(logger, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("setup conflict", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writeSetupProblem(&Views{logger: logger}, response, fmt.Errorf("wrapped: %w", domain.ErrAlreadyExists))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "Not found.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writeSetupProblem(&Views{logger: logger}, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("setup forbidden", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writeSetupProblem(&Views{logger: logger}, response, fmt.Errorf("wrapped: %w", domain.ErrForbidden))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "Not found.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writeSetupProblem(&Views{logger: logger}, response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})
}

type mediaReadFailureStub struct {
	imageService
	attachmentService
	err error
}

func (s mediaReadFailureStub) ImageContent(context.Context, int64) (domain.ImageData, error) {
	return domain.ImageData{}, s.err
}
func (s mediaReadFailureStub) AttachmentContent(context.Context, int64) (domain.AttachmentData, error) {
	return domain.AttachmentData{}, s.err
}

func TestMediaDownloadsUseLoggedTranslator(t *testing.T) {
	t.Parallel()
	t.Run("image", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		stub := mediaReadFailureStub{err: errors.New("read failed")}
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()

		ServeImage(stub, logger)(response, request)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Contains(t, logs.String(), "read failed")
	})

	t.Run("attachment", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		stub := mediaReadFailureStub{err: errors.New("read failed")}
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()

		ServeAttachment(stub, logger)(response, request)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Contains(t, logs.String(), "read failed")
	})
}

func TestLocalLoginTranslationPreservesHTML(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	views := &Views{logger: slog.New(slog.NewTextHandler(&logs, nil)), templates: map[string]*template.Template{
		"login": template.Must(template.New("public-layout").Parse(`{{.AuthError}} {{.AuthNext}}`)),
	}}
	response := httptest.NewRecorder()
	writeLocalLoginProblem(views, response, fmt.Errorf("login: %w", auth.ErrInvalidCredentials), "/pages/home")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), "Invalid username or password.")
	assert.Contains(t, response.Body.String(), "/pages/home")
	assert.Empty(t, logs.String())
}
