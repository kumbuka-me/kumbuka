package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPartialImportProblemReportsSavedPages(t *testing.T) {
	response := httptest.NewRecorder()
	called := writePartialImportProblem(slog.Default(), response, 2, errors.New("save failed"))

	assert.True(t, called)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Contains(t, response.Body.String(), "2 pages were saved")
}

func TestPartialImportProblemSkipsFailuresBeforeFirstSave(t *testing.T) {
	response := httptest.NewRecorder()
	called := writePartialImportProblem(slog.Default(), response, 0, errors.New("save failed"))

	assert.False(t, called)
	assert.Empty(t, response.Body.String())
}
