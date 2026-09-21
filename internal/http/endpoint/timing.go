package endpoint

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
)

// measurePageStage measures one page-handler stage when diagnostics are enabled. A request without a handler trace takes the cheap no-op path.
func measurePageStage(ctx context.Context, stage string) func() {
	return renderprofile.FromContext(ctx).Measure(stage)
}
