package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

// auditRepositoryStub provides controllable audit repository behavior for tests.
type auditRepositoryStub struct {
	// err configures the error returned by the test double.
	err error
}

func (s auditRepositoryStub) LogAudit(context.Context, int64, string, string, string, string) error {
	return s.err
}

func TestRecordAuditEventReportsPersistenceFailure(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	repository := auditRepositoryStub{err: errors.New("audit unavailable")}

	Record(
		context.Background(),
		logger,
		repository,
		42,
		"settings.updated",
		"settings",
		"application",
		"potentially sensitive detail",
	)

	logs := output.String()
	assert.Contains(t, logs, `"event":"audit_record_failed"`)
	assert.Contains(t, logs, `"action":"settings.updated"`)
	assert.Contains(t, logs, `"actor_id":42`)
	assert.Contains(t, logs, "audit unavailable")
	assert.NotContains(t, logs, "potentially sensitive detail")
}

func TestRecordAuditEventStaysQuietOnSuccess(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	Record(context.Background(), logger, auditRepositoryStub{}, 42, "settings.updated", "settings", "application", "detail")

	assert.Empty(t, output.String())
}
