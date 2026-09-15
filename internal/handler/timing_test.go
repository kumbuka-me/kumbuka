package handler

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageTimingLogsStructuredSummary(t *testing.T) {
	var logs bytes.Buffer
	views := &Views{}
	views.EnablePageTimings(slog.New(slog.NewTextHandler(&logs, nil)))

	request := httptest.NewRequest("GET", "/pages/example", nil)
	request, trace := views.startPageTiming(request)
	if trace == nil {
		t.Fatal("page timing trace was not created")
	}

	stop := measurePageStage(request.Context(), "view_data")
	stop()
	views.logPageTiming(trace, request, "example")

	output := logs.String()
	for _, want := range []string{
		"event=page_handler_timing",
		"method=GET",
		"path=/pages/example",
		"slug=example",
		"duration_ms=",
		"stages.view_data_ms=",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("timing log missing %q: %s", want, output)
		}
	}
}

func TestPageTimingDisabledDoesNotAttachTrace(t *testing.T) {
	views := &Views{}
	request := httptest.NewRequest("GET", "/pages/example", nil)

	profiled, trace := views.startPageTiming(request)
	if trace != nil {
		t.Fatal("page timing trace was created while diagnostics were disabled")
	}
	if profiled != request {
		t.Fatal("disabled page timing replaced the request")
	}
}
