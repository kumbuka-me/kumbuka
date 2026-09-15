package logging

import (
	"io"
	"log/slog"
)

// LogFormat defines a supported structured log format.
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// Setup creates a structured logger for the selected format and level.
func Setup(format LogFormat, debug bool, output io.Writer) *slog.Logger {
	options := &slog.HandlerOptions{}

	if debug {
		options.Level = slog.LevelDebug
	}

	var handler slog.Handler

	if format == LogFormatText {
		handler = slog.NewTextHandler(output, options)
	} else {
		handler = slog.NewJSONHandler(output, options)
	}

	return slog.New(handler)
}
