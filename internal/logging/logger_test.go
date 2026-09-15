package logging

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupLogger(t *testing.T) {
	t.Parallel()

	t.Run("JSON info", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer

		Setup(LogFormatJSON, false, &output).Info("ready",
			"event", "test",
		)

		assert.Contains(t, output.String(), `"msg":"ready"`)
		assert.Contains(t, output.String(), `"event":"test"`)
	})

	t.Run("text debug", func(t *testing.T) {
		t.Parallel()

		var output bytes.Buffer

		Setup(LogFormatText, true, &output).Debug("details")

		assert.Contains(t, output.String(), "level=DEBUG")
		assert.Contains(t, output.String(), `msg=details`)
	})
}
