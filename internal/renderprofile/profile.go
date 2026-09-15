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

type contextKey struct{}

var nextTraceID atomic.Uint64
var noopMeasure = func() {}

// Trace collects cumulative stage and WASM timing data for one top-level page render.
type Trace struct {
	id      uint64
	started time.Time

	mu              sync.Mutex
	stages          map[string]time.Duration
	wasmCalls       []WASMCall
	wasmSummary     WASMSummary
	droppedWASMCall int
}

// WASMCall contains one guest boundary timing. Durations are measured on the
// host and intentionally separate queueing, wire work, and guest execution.
type WASMCall struct {
	PluginID      string
	ModuleID      string
	Stage         string
	GateWait      time.Duration
	Instantiate   time.Duration
	Encode        time.Duration
	Allocate      time.Duration
	MemoryWrite   time.Duration
	Execute       time.Duration
	MemoryRead    time.Duration
	Decode        time.Duration
	Validate      time.Duration
	Total         time.Duration
	RequestBytes  int
	ResponseBytes int
	Failed        bool
}

// WASMSummary aggregates all guest calls, including calls omitted from the
// bounded per-call sample.
type WASMSummary struct {
	Calls         int
	RequestBytes  int
	ResponseBytes int
	GateWait      time.Duration
	Instantiate   time.Duration
	Encode        time.Duration
	Allocate      time.Duration
	MemoryWrite   time.Duration
	Execute       time.Duration
	MemoryRead    time.Duration
	Decode        time.Duration
	Validate      time.Duration
	Total         time.Duration
}

// Snapshot is an immutable copy suitable for structured logging.
type Snapshot struct {
	ID               uint64
	Duration         time.Duration
	Stages           map[string]time.Duration
	WASMCalls        []WASMCall
	WASMSummary      WASMSummary
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
