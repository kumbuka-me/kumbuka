package endpoint

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpectedPageUpdatedAt(t *testing.T) {
	t.Parallel()

	t.Run("accepts an existing page version", func(t *testing.T) {
		t.Parallel()

		expected := time.Date(2026, time.September, 19, 14, 30, 0, 123000000, time.UTC)
		actual, err := expectedPageUpdatedAt(strconv.FormatInt(expected.UnixNano(), 10), true)

		require.NoError(t, err)
		assert.True(t, actual.Equal(expected))
	})

	t.Run("allows no version for a new page", func(t *testing.T) {
		t.Parallel()

		actual, err := expectedPageUpdatedAt("", false)

		require.NoError(t, err)
		assert.True(t, actual.IsZero())
	})

	t.Run("requires a version for an existing page", func(t *testing.T) {
		t.Parallel()

		_, err := expectedPageUpdatedAt("", true)
		var validation *domain.ValidationError

		require.ErrorAs(t, err, &validation)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "expected_updated_at", validation.Fields[0].Field)
	})
}

func TestWritePageProblemReportsEditConflict(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	writePageProblem(slog.Default(), response, &domain.PageEditConflictError{CurrentRevision: 9})

	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "Revision 9 is now current")
	assert.Contains(t, response.Body.String(), "Your changes are still in the editor")
}
