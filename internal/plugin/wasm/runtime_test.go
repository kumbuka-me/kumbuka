package wasm_test

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/kumbuka/internal/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fixtureWASM = sync.OnceValues(func() ([]byte, error) {
	temporary, err := os.MkdirTemp("", "kumbuka-wasm-fixture-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	file := filepath.Join(temporary, "reactor.wasm")
	command := exec.Command("go", "build", "-buildmode=c-shared", "-ldflags=-s -w", "-o", file, "./testdata/reactor")
	command.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build test reactor: %w\n%s", err, output)
	}
	return os.ReadFile(file)
})

// archive handles the archive operation.
func archive(t *testing.T, wasmBytes []byte, stage string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{"README.md", []byte("# Fixture\n")},
		{"plugin.yaml", []byte("api_version: 1\nid: io.example.fixture\nname: Fixture\nversion: 1.0.0\nmodules:\n  - type: renderer-extension\n    id: fixture\n    stage: " + stage + "\npermissions: []\n")},
		{"plugin.wasm", wasmBytes},
	} {
		file, err := writer.Create(entry.name)
		require.NoError(t, err)
		_, err = file.Write(entry.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func declarativeArchive(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{"README.md", []byte("# Declarative\n")},
		{"plugin.yaml", []byte("api_version: 1\nid: io.example.declarative\nname: Declarative\nversion: 1.0.0\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\npermissions: []\n")},
	} {
		file, err := writer.Create(entry.name)
		require.NoError(t, err)
		_, err = file.Write(entry.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func TestDeclarativePackageLoadsWithoutWASM(t *testing.T) {
	pkg, err := pluginpackage.Read(declarativeArchive(t))
	require.NoError(t, err)
	require.False(t, pkg.Manifest().RequiresWASM())
	require.Empty(t, pkg.WASM())

	runtime, err := wasm.New(context.Background(), wasm.Limits{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close(context.Background())) })
	instance, err := runtime.Load(context.Background(), pkg)
	require.NoError(t, err)
	require.Len(t, instance.Contributions().MarkdownExtensions, 1)
	require.NoError(t, instance.Close(context.Background()))
}

func TestPluginInitializationTimingUsesDebugLogging(t *testing.T) {
	pkg, err := pluginpackage.Read(declarativeArchive(t))
	require.NoError(t, err)

	t.Run("debug", func(t *testing.T) {
		var output bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
		runtime, err := wasm.New(context.Background(), wasm.Limits{}, wasm.WithLogger(logger))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, runtime.Close(context.Background())) })

		instance, err := runtime.Load(context.Background(), pkg)
		require.NoError(t, err)
		require.NoError(t, instance.Close(context.Background()))

		log := output.String()
		assert.Contains(t, log, "level=DEBUG")
		assert.Contains(t, log, `msg="plugin initialized"`)
		assert.Contains(t, log, "event=plugin_initialized")
		assert.Contains(t, log, "plugin_id=io.example.declarative")
		assert.Contains(t, log, "wasm=false")
		assert.Contains(t, log, "duration=")
	})

	t.Run("info", func(t *testing.T) {
		var output bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&output, nil))
		runtime, err := wasm.New(context.Background(), wasm.Limits{}, wasm.WithLogger(logger))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, runtime.Close(context.Background())) })

		instance, err := runtime.Load(context.Background(), pkg)
		require.NoError(t, err)
		require.NoError(t, instance.Close(context.Background()))

		assert.Empty(t, output.String())
	})
}

// runtimeFixture handles the runtime fixture operation.
func runtimeFixture(t *testing.T, stage string, limits wasm.Limits) (plugin.Instance, []byte) {
	t.Helper()
	compiled, err := fixtureWASM()
	require.NoError(t, err)
	data := archive(t, compiled, stage)
	pkg, err := pluginpackage.Read(data)
	require.NoError(t, err)
	runtime, err := wasm.New(context.Background(), limits)
	require.NoError(t, err)
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	instance, err := runtime.Load(context.Background(), pkg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = instance.Close(context.Background()) })
	return instance, data
}

// TestBundledAndInstalledCalloutsUseSameRuntime verifies bundled and installed callouts use same runtime behavior.
func TestBundledAndInstalledCalloutsUseSameRuntime(t *testing.T) {
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	pkg, err := pluginpackage.Read(data)
	require.NoError(t, err)
	expectedVersion := pkg.Manifest().Version
	var outputs []string
	for _, source := range []plugin.Source{plugin.SourceBundled, plugin.SourceInstalled} {
		runtime, err := wasm.New(context.Background(), wasm.Limits{})
		require.NoError(t, err)
		registry := &plugin.Registry{}
		manager := plugin.NewManager(registry, runtime)
		t.Cleanup(func() { _ = manager.Close(context.Background()) })
		metadata, err := manager.Load(context.Background(), data, source)
		require.NoError(t, err)
		assert.Equal(t, source, metadata.Source)
		assert.Equal(t, expectedVersion, metadata.Manifest.Version)
		renderer := markdown.NewWithRegistry(registry)
		got, err := renderer.Render("!!! warning\n!!! note\n**Nested** [[Page]]\n")
		require.NoError(t, err)
		outputs = append(outputs, got)
		assert.Contains(t, got, `class="callout warning"`)
		assert.Contains(t, got, `class="callout note"`)
		assert.Contains(t, got, "<strong>Nested</strong>")
		assert.Contains(t, got, `href="/pages/page"`)
		snapshot := registry.Snapshot()
		require.NoError(t, manager.Unload(context.Background(), metadata.Manifest.ID))
		require.Empty(t, registry.Snapshot().Entries)
		_, err = snapshot.Entries[0].Contributions.Preprocessors[0].Preprocess(plugin.Context{}, "text")
		require.ErrorContains(t, err, "closed")
		got, err = renderer.Render("!!! note\nDisabled\n")
		require.NoError(t, err)
		assert.NotContains(t, got, `class="callout`)
	}
	assert.Equal(t, outputs[0], outputs[1])
}

// TestSandboxDeniesAmbientCapabilities verifies sandbox denies ambient capabilities behavior.
func TestSandboxDeniesAmbientCapabilities(t *testing.T) {
	instance, _ := runtimeFixture(t, "preprocess", wasm.Limits{})
	transform := instance.Contributions().Preprocessors[0]
	secret := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secret, []byte("host secret"), 0o600))
	t.Setenv("KUMBUKA_PLUGIN_TEST_SECRET", "host secret")
	environment, err := transform.Preprocess(plugin.Context{}, "environment")
	require.NoError(t, err)
	assert.Empty(t, environment)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	for _, source := range []string{"read:" + secret, "write:" + secret, "network:" + listener.Addr().String(), "process"} {
		got, err := transform.Preprocess(plugin.Context{}, source)
		require.NoError(t, err)
		assert.Equal(t, "denied", got, source)
	}
	remaining, err := os.ReadFile(secret)
	require.NoError(t, err)
	assert.Equal(t, "host secret", string(remaining))
}

// TestSandboxRejectsTrapsAndMalformedResultsAndRecovers verifies sandbox rejects traps and malformed results and recovers behavior.
func TestSandboxRejectsTrapsAndMalformedResultsAndRecovers(t *testing.T) {
	instance, _ := runtimeFixture(t, "preprocess", wasm.Limits{})
	transform := instance.Contributions().Preprocessors[0]
	for _, source := range []string{"trap", "bad-pointer", "oversized", "malformed", "trailing", "unknown-field", "grow"} {
		t.Run(source, func(t *testing.T) {
			_, err := transform.Preprocess(plugin.Context{}, source)
			require.Error(t, err)
			got, err := transform.Preprocess(plugin.Context{}, "healthy")
			require.NoError(t, err)
			assert.Equal(t, "healthy", got)
		})
	}
}

// TestSandboxTimeoutAndCancellation verifies sandbox timeout and cancellation behavior.
func TestSandboxTimeoutAndCancellation(t *testing.T) {
	instance, _ := runtimeFixture(t, "preprocess", wasm.Limits{CallTimeout: 100 * time.Millisecond})
	transform := instance.Contributions().Preprocessors[0]
	start := time.Now()
	_, err := transform.Preprocess(plugin.Context{}, "loop")
	require.Error(t, err)
	assert.Less(t, time.Since(start), 2*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = transform.Preprocess(plugin.Context{Context: ctx}, "loop")
	require.Error(t, err)
	got, err := transform.Preprocess(plugin.Context{}, "healthy")
	require.NoError(t, err)
	assert.Equal(t, "healthy", got)
}

// TestSandboxMemoryLimitAppliesAtInstantiation verifies sandbox memory limit applies at instantiation behavior.
func TestSandboxMemoryLimitAppliesAtInstantiation(t *testing.T) {
	compiled, err := fixtureWASM()
	require.NoError(t, err)
	pkg, err := pluginpackage.Read(archive(t, compiled, "preprocess"))
	require.NoError(t, err)
	runtime, err := wasm.New(context.Background(), wasm.Limits{MemoryPages: 1})
	require.NoError(t, err)
	defer func() { _ = runtime.Close(context.Background()) }()
	_, err = runtime.Load(context.Background(), pkg)
	require.Error(t, err)
}

// TestSandboxWireLimitAndPostprocessorContract verifies sandbox wire limit and postprocessor contract behavior.
func TestSandboxWireLimitAndPostprocessorContract(t *testing.T) {
	instance, _ := runtimeFixture(t, "postprocess", wasm.Limits{WireBytes: 1024})
	transform := instance.Contributions().Postprocessors[0]
	_, err := transform.Postprocess(plugin.Context{}, strings.Repeat("x", 1025))
	require.ErrorContains(t, err, "size limit")
	_, err = transform.Postprocess(plugin.Context{}, "recursive")
	require.ErrorContains(t, err, "fragment")
	got, err := transform.Postprocess(plugin.Context{}, "healthy")
	require.NoError(t, err)
	assert.Equal(t, "healthy", got)
}

// TestWASMOutputCannotBypassSanitizer verifies wasmoutput cannot bypass sanitizer behavior.
func TestWASMOutputCannotBypassSanitizer(t *testing.T) {
	instance, _ := runtimeFixture(t, "preprocess", wasm.Limits{})
	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "fixture", Name: "Fixture"}, instance.Contributions()))
	renderer := markdown.NewWithRegistry(registry)
	got, err := renderer.Render("unsafe")
	require.NoError(t, err)
	assert.Contains(t, got, "safe")
	assert.NotContains(t, got, "<script")
	assert.NotContains(t, got, "javascript:")
	_, err = renderer.Render("recursive")
	require.ErrorContains(t, err, "nesting limit")
}

func TestRenderTimingsIncludeWASMBoundary(t *testing.T) {
	instance, _ := runtimeFixture(t, "preprocess", wasm.Limits{})
	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "fixture", Name: "Fixture"}, instance.Contributions()))
	renderer := markdown.NewWithRegistry(registry)

	var logs bytes.Buffer
	renderer.EnableRenderTimings(slog.New(slog.NewJSONHandler(&logs, nil)))

	_, err := renderer.Render("healthy")
	require.NoError(t, err)

	output := logs.String()
	assert.Contains(t, output, `"event":"wasm_render_timing"`)
	assert.Contains(t, output, `"plugin_id":"io.example.fixture"`)
	assert.Contains(t, output, `"module_id":"fixture"`)
	assert.Contains(t, output, `"request_bytes":`)
	assert.Contains(t, output, `"response_bytes":`)
	assert.Contains(t, output, `"guest_execute_ms":`)
	assert.Contains(t, output, `"event":"render_timing"`)
	assert.Contains(t, output, `"wasm_calls":1`)
}

// TestWASMRequestsAreIsolatedAndSerialized verifies wasmrequests are isolated and serialized behavior.
func TestWASMRequestsAreIsolatedAndSerialized(t *testing.T) {
	instance, _ := runtimeFixture(t, "preprocess", wasm.Limits{})
	transform := instance.Contributions().Preprocessors[0]
	var wg sync.WaitGroup
	for index := range 12 {
		wg.Go(func() {
			source := fmt.Sprintf("request-%d", index)
			got, err := transform.Preprocess(plugin.Context{}, source)
			assert.NoError(t, err)
			assert.Equal(t, source, got)
		})
	}
	wg.Wait()
}

// TestRuntimeRejectsMissingABI verifies runtime rejects missing abi behavior.
func TestRuntimeRejectsMissingABI(t *testing.T) {
	pkg, err := pluginpackage.Read(archive(t, []byte{0, 'a', 's', 'm', 1, 0, 0, 0}, "preprocess"))
	require.NoError(t, err)
	runtime, err := wasm.New(context.Background(), wasm.Limits{})
	require.NoError(t, err)
	defer func() { _ = runtime.Close(context.Background()) }()
	_, err = runtime.Load(context.Background(), pkg)
	require.ErrorContains(t, err, "memory")
}
