package endpoint

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
	"github.com/stretchr/testify/assert"
)

func TestMeasurePageStageWithoutRequestTraceIsNoop(t *testing.T) {
	stop := measurePageStage(context.Background(), "view_data")
	stop()
}

func TestMeasurePageStageUsesSharedRequestTrace(t *testing.T) {
	trace := renderprofile.New()
	ctx := renderprofile.WithContext(context.Background(), trace)

	stop := measurePageStage(ctx, "view_data")
	stop()

	assert.Contains(t, trace.Snapshot().Stages, "view_data")
}
