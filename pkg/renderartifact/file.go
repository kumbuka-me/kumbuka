// Package renderartifact provides trusted host adapters for SDK passive artifacts.
package renderartifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	sdk "github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/renderartifact"
)

// FileCapability returns a request-scoped renderer artifact writer for a staged static site.
// It never chooses filenames from untrusted plugin strings.
func FileCapability(directory, basePath string) plugin.Capability {
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

		digest := renderartifact.Digest(input.Data)
		name := digest + ".svg"
		dest := filepath.Join(directory, "assets", "rendered", name)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, fmt.Errorf("create artifact directory: %w", err)
		}
		// Content-addressed writes are idempotent. Atomic renames keep parallel builds safe.
		if err := writeArtifact(dest, input.Data); err != nil {
			return nil, err
		}
		prefix := "/" + strings.Trim(basePath, "/")
		if prefix == "/" {
			prefix = ""
		}
		return sdk.RenderedArtifactRef{URL: prefix + "/" + path.Join("assets", "rendered", name)}, nil
	}
}

// writeArtifact atomically publishes one passive generated file.
func writeArtifact(destination string, data []byte) error {
	if _, err := os.Stat(destination); err == nil {
		return nil
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".artifact-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), destination)
}

// validateSVG applies the SDK's conservative passive SVG policy.
func validateSVG(data []byte) error {
	return renderartifact.Validate(renderartifact.SVGMediaType, data)
}
