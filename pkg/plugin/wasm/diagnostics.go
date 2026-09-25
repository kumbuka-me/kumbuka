package wasm

import (
	"strings"
	"unicode/utf8"
)

const (
	maxGuestDiagnosticBytes     = 16 << 10
	maxGuestDiagnosticLineBytes = 1024
)

// guestDiagnostics captures a bounded prefix of guest stderr for crash diagnostics.
type guestDiagnostics struct {
	data []byte
}

// Reset clears diagnostics captured by the previous guest invocation.
func (d *guestDiagnostics) Reset() {
	d.data = d.data[:0]
}

// Write captures at most maxGuestDiagnosticBytes while reporting the full write as consumed.
func (d *guestDiagnostics) Write(value []byte) (int, error) {
	written := len(value)
	remaining := maxGuestDiagnosticBytes - len(d.data)
	if remaining <= 0 {
		return written, nil
	}
	if len(value) > remaining {
		value = value[:remaining]
	}
	d.data = append(d.data, value...)
	return written, nil
}

// Summary returns the first useful guest error line without the runtime stack dump.
func (d *guestDiagnostics) Summary() string {
	for _, line := range strings.Split(string(d.data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "panic:") || strings.HasPrefix(line, "fatal error:") || strings.HasPrefix(line, "runtime:") {
			return boundedDiagnosticLine(line)
		}
	}
	return ""
}

// boundedDiagnosticLine limits one guest-controlled diagnostic without splitting UTF-8.
func boundedDiagnosticLine(line string) string {
	line = strings.ToValidUTF8(line, "�")
	if len(line) <= maxGuestDiagnosticLineBytes {
		return line
	}
	line = line[:maxGuestDiagnosticLineBytes]
	for len(line) > 0 && !utf8.ValidString(line) {
		line = line[:len(line)-1]
	}
	return strings.TrimSpace(line)
}
