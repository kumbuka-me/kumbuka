package endpoint

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReviewLineRangeDefaultsEnd verifies a single submitted line becomes a one-line inclusive range.
func TestReviewLineRangeDefaultsEnd(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	start, end, ok := reviewLineRange(response, "12", "")

	assert.True(t, ok)
	assert.Equal(t, 12, start)
	assert.Equal(t, 12, end)
}
