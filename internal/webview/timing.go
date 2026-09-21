package webview

import (
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
)

// EnablePageTimings enables opt-in timing diagnostics for page rendering.
func (v *Views) EnablePageTimings(logger *slog.Logger) {
	v.pageTimingLogger = logger
}

// StartPageTiming attaches one handler trace to the request so nested view-data loading can contribute detailed stages without changing service interfaces.
func (v *Views) StartPageTiming(r *http.Request) (*http.Request, *renderprofile.Trace) {
	if v == nil || v.pageTimingLogger == nil {
		return r, nil
	}

	trace := renderprofile.New()
	return r.WithContext(renderprofile.WithContext(r.Context(), trace)), trace
}

// LogPageTiming writes one summary after a page handler returns.
func (v *Views) LogPageTiming(trace *renderprofile.Trace, r *http.Request, slug string) {
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
		stageAttributes = append(stageAttributes, name+"_ms", durationMilliseconds(snapshot.Stages[name]))
	}

	args := []any{
		"event", "page_handler_timing",
		"profile_id", snapshot.ID,
		"method", r.Method,
		"path", r.URL.Path,
		"slug", slug,
		"duration_ms", durationMilliseconds(snapshot.Duration),
	}
	if len(stageAttributes) != 0 {
		args = append(args, slog.Group("stages", stageAttributes...))
	}

	v.pageTimingLogger.Info("page handler timing", args...)
}

// durationMilliseconds converts a page timing duration to milliseconds.
func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
