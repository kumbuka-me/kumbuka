package renderartifact

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	quickjs "github.com/go-quickjs/go-quickjs"
	"github.com/kumbuka-me/sdk/renderartifact"
)

const renderTimeout = 15 * time.Second

// Engine evaluates only scripts extracted from a verified plugin package.
// It has no filesystem, network, process or DOM APIs.
type Engine struct {
	// mu serializes calls into one JavaScript execution context.
	mu sync.Mutex
	// runtime owns the reusable, isolated JavaScript environment.
	runtime *quickjs.Runtime
	// digest identifies the immutable script already loaded in runtime.
	digest string
}

// Render evaluates a packaged renderer and returns its passive SVG result.
func (e *Engine) Render(ctx context.Context, script []byte, source, theme string) ([]byte, error) {
	if len(script) < 32 || len(script) > 8<<20 || len(source) > 64<<10 {
		return nil, errors.New("invalid artifact script or source size")
	}
	ctx, cancel := context.WithTimeout(ctx, renderTimeout)
	defer cancel()
	e.mu.Lock()
	defer e.mu.Unlock()

	digest := renderartifact.Digest(script)
	if e.runtime == nil || e.digest != digest {
		if e.runtime != nil {
			e.runtime.Close() // nolint:errcheck
		}
		e.runtime = quickjs.New(quickjs.WithMemoryLimit(256 << 20))
		if _, err := e.runtime.EvalFileContext(ctx, "plugin-headless.js", string(script)); err != nil {
			e.runtime.Close() // nolint:errcheck
			e.runtime = nil
			return nil, fmt.Errorf("initialize plugin renderer: %w", err)
		}
		e.digest = digest
	}
	if err := e.runtime.Set("kumbukaSource", source); err != nil {
		return nil, err
	}
	if err := e.runtime.Set("kumbukaTheme", theme); err != nil {
		return nil, err
	}
	result, err := e.runtime.EvalContext(ctx, `globalThis.kumbukaRenderArtifact(kumbukaSource, kumbukaTheme)`)
	if err != nil {
		return nil, fmt.Errorf("execute plugin renderer: %w", err)
	}
	result, err = result.Await(ctx)
	if err != nil {
		return nil, fmt.Errorf("await plugin renderer: %w", err)
	}
	data := []byte(result.String())
	if err := ValidateSVG(data); err != nil {
		return nil, err
	}
	return data, nil
}

// Close releases the engine's JavaScript runtime.
func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runtime != nil {
		e.runtime.Close() // nolint:errcheck
		e.runtime = nil
	}
}

// ValidateSVG checks generated image markup against the shared SDK policy.
func ValidateSVG(data []byte) error { return validateSVG(data) }
