package endpoint

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

type failingPageViewRecorder struct{}

func (failingPageViewRecorder) RecordView(context.Context, string, int64) error {
	return errors.New("view history unavailable")
}

func TestRecordPageViewReportsPersistenceFailure(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	recordPageView(context.Background(), logger, failingPageViewRecorder{}, "guide", 42)

	logs := output.String()
	assert.Contains(t, logs, `"event":"page_view_record_failed"`)
	assert.Contains(t, logs, `"slug":"guide"`)
	assert.Contains(t, logs, `"user_id":42`)
	assert.Contains(t, logs, "view history unavailable")
}
