package renderartifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	sdk "github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/renderartifact"
)

// PageStore persists validated images against one immutable page revision.
type PageStore interface {
	// PutPageSVG stores an image only while the specified page revision remains current.
	PutPageSVG(context.Context, int64, time.Time, []byte) (string, error)
}

// PageCapability binds generic artifact writes to an authorized page and its revision.
// The caller must authorize that page before granting this capability to a plugin.
func PageCapability(store PageStore, pageID int64, revision time.Time, prefix string) plugin.Capability {
	return func(ctx context.Context, payload json.RawMessage) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var input sdk.RenderedArtifactWrite
		if err := json.Unmarshal(payload, &input); err != nil {
			return nil, errors.New("invalid render artifact request")
		}
		if err := renderartifact.Validate(input.MediaType, input.Data); err != nil {
			return nil, err
		}
		digest, err := store.PutPageSVG(ctx, pageID, revision, input.Data)
		if err != nil {
			return nil, err
		}
		return sdk.RenderedArtifactRef{
			URL: fmt.Sprintf("%s/api/render-artifacts/%d/%s.svg", prefix, pageID, digest),
		}, nil
	}
}
