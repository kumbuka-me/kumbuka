// Package renderprofile collects opt-in render timing diagnostics without
// coupling the Markdown renderer and WASM runtime to each other's logging.
package renderprofile

import (
	"context"
	"maps"
	"sync"
	"sync/atomic"
	"time"
)

const maxRecordedWASMCalls = 256

// contextKey is the private type used for request-context values.
type contextKey struct{}

var nextTraceID atomic.Uint64
var noopMeasure = func() {}

// Trace collects cumulative stage and WASM timing data for one top-level page render.
type Trace struct {
	// id identifies trace.
	id uint64
	// started stores the started value used by trace.
	started time.Time

	// mu protects concurrent access to trace.
	mu sync.Mutex
	// stages maps keys to stages values used by trace.
	stages map[string]time.Duration
	// wasmCalls contains the wasm calls associated with trace.
	wasmCalls []WASMCall
	// wasmSummary stores the wasm summary value used by trace.
	wasmSummary WASMSummary
	// droppedWASMCall stores the dropped WASM call value used by trace.
	droppedWASMCall int
}

// WASMCall contains one guest boundary timing. Durations are measured on the
// host and intentionally separate queueing, wire work, and guest execution.
type WASMCall struct {
	// PluginID identifies the plugin associated with WASM call.
	PluginID string
	// ModuleID identifies the module associated with WASM call.
	ModuleID string
	// Stage stores the stage value used by WASM call.
	Stage string
	// GateWait stores the gate wait value used by WASM call.
	GateWait time.Duration
	// Instantiate stores the instantiate value used by WASM call.
	Instantiate time.Duration
	// Encode stores the encode value used by WASM call.
	Encode time.Duration
	// Allocate stores the allocate value used by WASM call.
	Allocate time.Duration
	// MemoryWrite stores the memory write value used by WASM call.
	MemoryWrite time.Duration
	// Execute stores the execute value used by WASM call.
	Execute time.Duration
	// MemoryRead stores the memory read value used by WASM call.
	MemoryRead time.Duration
	// Decode stores the decode value used by WASM call.
	Decode time.Duration
	// Validate stores the validate value used by WASM call.
	Validate time.Duration
	// Total stores the total value used by WASM call.
	Total time.Duration
	// RequestBytes stores the request bytes value used by WASM call.
	RequestBytes int
	// ResponseBytes stores the response bytes value used by WASM call.
	ResponseBytes int
	// Failed reports whether failed applies to WASM call.
	Failed bool
}

// WASMSummary aggregates all guest calls, including calls omitted from the
// bounded per-call sample.
type WASMSummary struct {
	// Calls stores the calls value used by WASM summary.
	Calls int
	// RequestBytes stores the request bytes value used by WASM summary.
	RequestBytes int
	// ResponseBytes stores the response bytes value used by WASM summary.
	ResponseBytes int
	// GateWait stores the gate wait value used by WASM summary.
	GateWait time.Duration
	// Instantiate stores the instantiate value used by WASM summary.
	Instantiate time.Duration
	// Encode stores the encode value used by WASM summary.
	Encode time.Duration
	// Allocate stores the allocate value used by WASM summary.
	Allocate time.Duration
	// MemoryWrite stores the memory write value used by WASM summary.
	MemoryWrite time.Duration
	// Execute stores the execute value used by WASM summary.
	Execute time.Duration
	// MemoryRead stores the memory read value used by WASM summary.
	MemoryRead time.Duration
	// Decode stores the decode value used by WASM summary.
	Decode time.Duration
	// Validate stores the validate value used by WASM summary.
	Validate time.Duration
	// Total stores the total value used by WASM summary.
	Total time.Duration
}

// Snapshot is an immutable copy suitable for structured logging.
type Snapshot struct {
	// ID identifies snapshot.
	ID uint64
	// Duration stores the duration value used by snapshot.
	Duration time.Duration
	// Stages maps keys to stages values used by snapshot.
	Stages map[string]time.Duration
	// WASMCalls contains the WASM calls associated with snapshot.
	WASMCalls []WASMCall
	// WASMSummary stores the WASM summary value used by snapshot.
	WASMSummary WASMSummary
	// DroppedWASMCalls stores the dropped WASM calls value used by snapshot.
	DroppedWASMCalls int
}

// New starts one render trace.
func New() *Trace {
	return &Trace{
		id:      nextTraceID.Add(1),
		started: time.Now(),
		stages:  make(map[string]time.Duration),
	}
}

// WithContext makes trace available to lower-level guest invocation code.
func WithContext(ctx context.Context, trace *Trace) context.Context {
	if trace == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, trace)
}

// FromContext returns the render trace attached to ctx, if any.
func FromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(contextKey{}).(*Trace)
	return trace
}

// Measure returns a completion function that adds elapsed time to one stage.
// Calling it on a nil trace is intentionally a cheap no-op.
func (t *Trace) Measure(stage string) func() {
	if t == nil {
		return noopMeasure
	}
	started := time.Now()
	return func() {
		t.addStage(stage, time.Since(started))
	}
}

// addStage adds one measured stage to the render profile.
func (t *Trace) addStage(stage string, duration time.Duration) {
	if t == nil || stage == "" {
		return
	}
	t.mu.Lock()
	t.stages[stage] += duration
	t.mu.Unlock()
}

// RecordWASM records one guest boundary measurement. Aggregate values include
// every call while detailed call storage is bounded for pathological renders.
func (t *Trace) RecordWASM(call WASMCall) {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.wasmSummary.Calls++
	t.wasmSummary.RequestBytes += call.RequestBytes
	t.wasmSummary.ResponseBytes += call.ResponseBytes
	t.wasmSummary.GateWait += call.GateWait
	t.wasmSummary.Instantiate += call.Instantiate
	t.wasmSummary.Encode += call.Encode
	t.wasmSummary.Allocate += call.Allocate
	t.wasmSummary.MemoryWrite += call.MemoryWrite
	t.wasmSummary.Execute += call.Execute
	t.wasmSummary.MemoryRead += call.MemoryRead
	t.wasmSummary.Decode += call.Decode
	t.wasmSummary.Validate += call.Validate
	t.wasmSummary.Total += call.Total

	if len(t.wasmCalls) < maxRecordedWASMCalls {
		t.wasmCalls = append(t.wasmCalls, call)
		return
	}
	t.droppedWASMCall++
}

// Snapshot copies the trace while rendering may still be unwinding.
func (t *Trace) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	return Snapshot{
		ID:               t.id,
		Duration:         time.Since(t.started),
		Stages:           maps.Clone(t.stages),
		WASMCalls:        append([]WASMCall(nil), t.wasmCalls...),
		WASMSummary:      t.wasmSummary,
		DroppedWASMCalls: t.droppedWASMCall,
	}
}
