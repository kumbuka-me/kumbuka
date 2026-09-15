package renderprofile

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceCollectsStageAndWASMSummary(t *testing.T) {
	trace := New()
	ctx := WithContext(context.Background(), trace)
	require.Same(t, trace, FromContext(ctx))

	stop := trace.Measure("goldmark")
	stop()
	trace.RecordWASM(WASMCall{
		PluginID:      "io.example.plugin",
		ModuleID:      "render",
		Stage:         "preprocess",
		GateWait:      time.Millisecond,
		Execute:       2 * time.Millisecond,
		Total:         4 * time.Millisecond,
		RequestBytes:  120,
		ResponseBytes: 80,
	})

	snapshot := trace.Snapshot()
	assert.NotZero(t, snapshot.ID)
	assert.Greater(t, snapshot.Duration, time.Duration(0))
	assert.Contains(t, snapshot.Stages, "goldmark")
	require.Len(t, snapshot.WASMCalls, 1)
	assert.Equal(t, 1, snapshot.WASMSummary.Calls)
	assert.Equal(t, 120, snapshot.WASMSummary.RequestBytes)
	assert.Equal(t, 80, snapshot.WASMSummary.ResponseBytes)
	assert.Equal(t, 2*time.Millisecond, snapshot.WASMSummary.Execute)
	assert.Equal(t, 4*time.Millisecond, snapshot.WASMSummary.Total)
}

func TestTraceBoundsDetailedWASMCalls(t *testing.T) {
	trace := New()
	for range maxRecordedWASMCalls + 2 {
		trace.RecordWASM(WASMCall{Total: time.Millisecond})
	}

	snapshot := trace.Snapshot()
	assert.Len(t, snapshot.WASMCalls, maxRecordedWASMCalls)
	assert.Equal(t, 2, snapshot.DroppedWASMCalls)
	assert.Equal(t, maxRecordedWASMCalls+2, snapshot.WASMSummary.Calls)
	assert.Equal(t, time.Duration(maxRecordedWASMCalls+2)*time.Millisecond, snapshot.WASMSummary.Total)
}
