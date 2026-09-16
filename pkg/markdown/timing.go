package markdown

import (
	"log/slog"
	"sort"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/renderprofile"
)

// EnableRenderTimings enables opt-in structured render profiling. Call it
// during application setup, before the renderer is served concurrently.
func (r *Renderer) EnableRenderTimings(logger *slog.Logger) {
	r.timingLogger = logger
}

func (r *Renderer) logRenderTimings(trace *renderprofile.Trace, sourceBytes, outputBytes int, renderErr error) {
	if r.timingLogger == nil || trace == nil {
		return
	}

	snapshot := trace.Snapshot()
	for index, call := range snapshot.WASMCalls {
		r.timingLogger.Info(
			"WASM render timing",
			"event", "wasm_render_timing",
			"render_id", snapshot.ID,
			"call", index+1,
			"plugin_id", call.PluginID,
			"module_id", call.ModuleID,
			"stage", call.Stage,
			"failed", call.Failed,
			"duration_ms", durationMilliseconds(call.Total),
			"gate_wait_ms", durationMilliseconds(call.GateWait),
			"instantiate_ms", durationMilliseconds(call.Instantiate),
			"encode_ms", durationMilliseconds(call.Encode),
			"allocate_ms", durationMilliseconds(call.Allocate),
			"memory_write_ms", durationMilliseconds(call.MemoryWrite),
			"guest_execute_ms", durationMilliseconds(call.Execute),
			"memory_read_ms", durationMilliseconds(call.MemoryRead),
			"decode_ms", durationMilliseconds(call.Decode),
			"validate_ms", durationMilliseconds(call.Validate),
			"request_bytes", call.RequestBytes,
			"response_bytes", call.ResponseBytes,
		)
	}

	stageNames := make([]string, 0, len(snapshot.Stages))
	for name := range snapshot.Stages {
		stageNames = append(stageNames, name)
	}
	sort.Strings(stageNames)
	stageAttributes := make([]any, 0, len(stageNames)*2)
	for _, name := range stageNames {
		stageAttributes = append(stageAttributes, name+"_ms", durationMilliseconds(snapshot.Stages[name]))
	}

	summary := snapshot.WASMSummary
	args := []any{
		"event", "render_timing",
		"render_id", snapshot.ID,
		"failed", renderErr != nil,
		"duration_ms", durationMilliseconds(snapshot.Duration),
		"source_bytes", sourceBytes,
		"output_bytes", outputBytes,
		"wasm_calls", summary.Calls,
		"wasm_calls_logged", len(snapshot.WASMCalls),
		"wasm_calls_dropped", snapshot.DroppedWASMCalls,
		"wasm_request_bytes", summary.RequestBytes,
		"wasm_response_bytes", summary.ResponseBytes,
		"wasm_total_ms", durationMilliseconds(summary.Total),
		"wasm_gate_wait_ms", durationMilliseconds(summary.GateWait),
		"wasm_instantiate_ms", durationMilliseconds(summary.Instantiate),
		"wasm_encode_ms", durationMilliseconds(summary.Encode),
		"wasm_allocate_ms", durationMilliseconds(summary.Allocate),
		"wasm_memory_write_ms", durationMilliseconds(summary.MemoryWrite),
		"wasm_guest_execute_ms", durationMilliseconds(summary.Execute),
		"wasm_memory_read_ms", durationMilliseconds(summary.MemoryRead),
		"wasm_decode_ms", durationMilliseconds(summary.Decode),
		"wasm_validate_ms", durationMilliseconds(summary.Validate),
	}
	if len(stageAttributes) != 0 {
		args = append(args, slog.Group("stages", stageAttributes...))
	}

	r.timingLogger.Info("render timing", args...)
}

func durationMilliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}
