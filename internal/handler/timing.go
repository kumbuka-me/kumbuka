package handler

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/renderprofile"
)

// EnablePageTimings enables opt-in timing diagnostics for the page HTTP
// handler and its shared view-data work. Call it during application setup.
func (v *Views) EnablePageTimings(logger *slog.Logger) {
	v.pageTimingLogger = logger
}

// startPageTiming attaches one handler trace to the request so nested view-data
// loading can contribute detailed stages without changing service interfaces.
func (v *Views) startPageTiming(r *http.Request) (*http.Request, *renderprofile.Trace) {
	if v == nil || v.pageTimingLogger == nil {
		return r, nil
	}

	trace := renderprofile.New()
	return r.WithContext(renderprofile.WithContext(r.Context(), trace)), trace
}

// measurePageStage measures one page-handler stage when diagnostics are
// enabled. A request without a handler trace takes the cheap no-op path.
func measurePageStage(ctx context.Context, stage string) func() {
	return renderprofile.FromContext(ctx).Measure(stage)
}

// logPageTiming writes one summary after ViewPage returns. Its duration starts
// at handler entry, so comparing it with request_complete isolates latency in
// routing/auth/middleware that happens outside the page handler.
func (v *Views) logPageTiming(trace *renderprofile.Trace, r *http.Request, slug string) {
	if v == nil || v.pageTimingLogger == nil || trace == nil {
		return
	}

	snapshot := trace.Snapshot()
	stageNames := make([]string, 0, len(snapshot.Stages))
	for name := range snapshot.Stages {
		stageNames = append(stageNames, name)
	}
	sort.Strings(stageNames)

	stageAttributes := make([]any, 0, len(stageNames)*2)
	for _, name := range stageNames {
		stageAttributes = append(stageAttributes, name+"_ms", pageDurationMilliseconds(snapshot.Stages[name]))
	}

	args := []any{
		"event", "page_handler_timing",
		"profile_id", snapshot.ID,
		"method", r.Method,
		"path", r.URL.Path,
		"slug", slug,
		"duration_ms", pageDurationMilliseconds(snapshot.Duration),
	}
	if len(stageAttributes) != 0 {
		args = append(args, slog.Group("stages", stageAttributes...))
	}

	v.pageTimingLogger.Info("page handler timing", args...)
}

// pageDurationMilliseconds converts a page timing duration to milliseconds.
func pageDurationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
