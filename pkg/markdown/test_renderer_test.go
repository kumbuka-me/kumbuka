package markdown

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/stretchr/testify/require"
)

// sharedRendererEntry provides test state for shared renderer entry behavior.
type sharedRendererEntry struct {
	// ready configures or records the ready value used by the fixture.
	ready chan struct{}
	// renderer configures or records the renderer value used by the fixture.
	renderer *Renderer
	// manager provides the manager dependency used by the fixture.
	manager *plugin.Manager
	// err configures the error returned by the test double.
	err error
}

var sharedTestRenderers = struct {
	sync.Mutex
	entries map[string]*sharedRendererEntry
}{entries: make(map[string]*sharedRendererEntry)}

// testRenderer returns a cheap core renderer when no plugins are requested.
// Plugin-backed renderers are shared by exact plugin set because most Markdown
// tests only inspect rendering behavior. Tests that mutate plugin lifecycle or
// settings must use isolatedTestRenderer instead.
func testRenderer(t testing.TB, names ...string) *Renderer {
	t.Helper()
	if len(names) == 0 {
		return NewWithRegistry(nil)
	}

	key := strings.Join(names, "\x00")

	sharedTestRenderers.Lock()
	entry, ok := sharedTestRenderers.entries[key]
	if !ok {
		entry = &sharedRendererEntry{ready: make(chan struct{})}
		sharedTestRenderers.entries[key] = entry
	}
	sharedTestRenderers.Unlock()

	if !ok {
		entry.renderer, entry.manager, entry.err = newPluginTestRenderer(names...)
		close(entry.ready)
	} else {
		<-entry.ready
	}

	require.NoError(t, entry.err)
	require.NotNil(t, entry.renderer)
	return entry.renderer
}

// isolatedTestRenderer returns a fresh plugin renderer for tests that mutate
// plugin manager state, registry contents, or plugin settings.
func isolatedTestRenderer(t testing.TB, names ...string) *Renderer {
	t.Helper()

	renderer, manager, err := newPluginTestRenderer(names...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })
	return renderer
}

func newPluginTestRenderer(names ...string) (*Renderer, *plugin.Manager, error) {
	ctx := context.Background()
	runtime, err := wasm.New(ctx, wasm.Limits{InitTimeout: 30 * time.Second}, wasm.WithPermissions("pages:read", "pages:content", "browser:render"), wasm.WithInterpreter())
	if err != nil {
		return nil, nil, err
	}

	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime)
	archives := make([][]byte, 0, len(names))
	for _, name := range names {
		archive, readErr := plugins.Packages.ReadFile(name + ".kumbukaplugin")
		if readErr != nil {
			_ = manager.Close(context.Background())
			return nil, nil, fmt.Errorf("read test plugin %s: %w", name, readErr)
		}
		archives = append(archives, archive)
	}

	if err := manager.Bootstrap(ctx, archives); err != nil {
		_ = manager.Close(context.Background())
		return nil, nil, err
	}

	return NewWithManager(registry, manager), manager, nil
}

func TestMain(m *testing.M) {
	code := m.Run()

	sharedTestRenderers.Lock()
	entries := make([]*sharedRendererEntry, 0, len(sharedTestRenderers.entries))
	for _, entry := range sharedTestRenderers.entries {
		entries = append(entries, entry)
	}
	sharedTestRenderers.Unlock()

	for _, entry := range entries {
		<-entry.ready
		if entry.manager == nil {
			continue
		}
		if err := entry.manager.Close(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "close shared Markdown test renderer: %v\n", err)
			code = 1
		}
	}

	os.Exit(code)
}
