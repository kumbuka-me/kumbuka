package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/ascii"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/renderartifact"
	sdk "github.com/kumbuka-me/sdk"
)

// renderArtifact evaluates a caller-owned, packaged headless renderer and publishes a validated image.
func (r *Runtime) renderArtifact(ctx context.Context, caller *Instance, data json.RawMessage) (any, error) {
	var request sdk.RenderedArtifactRender
	if err := decode(data, &request); err != nil {
		return nil, errors.New("invalid artifact render request")
	}
	if !validArtifactScriptName(request.Script) || len(request.Source) == 0 || len(request.Source) > 64<<10 || len(request.Theme) > 64 {
		return nil, errors.New("invalid artifact render parameters")
	}
	code, err := caller.packageAssets.Asset(request.Script)
	if err != nil {
		return nil, fmt.Errorf("load plugin renderer asset: %w", err)
	}
	if caller.artifactEngine == nil {
		caller.artifactEngine = &renderartifact.Engine{}
	}
	svg, err := caller.artifactEngine.Render(ctx, code, request.Source, request.Theme)
	if err != nil {
		return nil, err
	}

	scope, _ := ctx.Value(capabilitiesKey{}).(map[string]plugin.Capability)
	publish := scope["artifacts.put"]
	if publish == nil {
		return nil, errors.New("artifact storage unavailable in render context")
	}
	payload, err := json.Marshal(sdk.RenderedArtifactWrite{MediaType: "image/svg+xml", Data: svg})
	if err != nil {
		return nil, err
	}
	return publish(ctx, payload)
}

// validArtifactScriptName accepts only simple package-owned JavaScript assets.
func validArtifactScriptName(name string) bool {
	if len(name) < 4 || len(name) > 80 || !strings.HasSuffix(name, ".js") {
		return false
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		if ascii.IsLowercaseLetter(c) || ascii.IsDigit(c) ||
			c == '-' || c == '_' || c == '.' {
			continue
		}
		return false
	}

	return !strings.Contains(name, "..")
}
