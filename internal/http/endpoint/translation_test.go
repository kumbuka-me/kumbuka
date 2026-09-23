package endpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
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

	t.Run("setup conflict", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writeSetupProblem(testHandlerViewsWithLogger(t, logger, webview.RuntimeInfo{}), response, fmt.Errorf("wrapped: %w", domain.ErrAlreadyExists))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "Not found.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writeSetupProblem(testHandlerViewsWithLogger(t, logger, webview.RuntimeInfo{}), response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})

	t.Run("setup forbidden", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		writeSetupProblem(testHandlerViewsWithLogger(t, logger, webview.RuntimeInfo{}), response, fmt.Errorf("wrapped: %w", domain.ErrForbidden))
		assert.Equal(t, 404, response.Code)
		assert.Contains(t, response.Body.String(), "Not found.")
		assert.Empty(t, logs.String())
		failure := errors.New("private persistence details")
		response = httptest.NewRecorder()
		writeSetupProblem(testHandlerViewsWithLogger(t, logger, webview.RuntimeInfo{}), response, failure)
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), failure.Error())
		assert.Contains(t, logs.String(), failure.Error())
	})
}

// mediaReadFailureStub provides controllable media read failure behavior for tests.
type mediaReadFailureStub struct {
	imageService
	attachmentService
	// err configures the error returned by the test double.
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
	views := testHandlerViewsWithOverrides(
		t,
		slog.New(slog.NewTextHandler(&logs, nil)),
		webview.RuntimeInfo{},
		map[string]string{
			"templates/public_layout.gohtml": `{{ define "public-layout" }}{{ .AuthError }} {{ .AuthNext }}{{ end }}`,
		},
	)
	response := httptest.NewRecorder()
	writeLocalLoginProblem(views, response, fmt.Errorf("login: %w", auth.ErrInvalidCredentials), "/pages/home")
	assert.Equal(t, http.StatusUnauthorized, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), "Invalid username or password.")
	assert.Contains(t, response.Body.String(), "/pages/home")
	assert.Empty(t, logs.String())
}
