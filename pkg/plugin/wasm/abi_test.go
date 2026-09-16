package wasm_test

import (
	"context"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/require"
)

// Minimal reactors keep malformed-ABI tests independent of Go's compiler.
// The four function types mirror the documented ABI; bodies are explicit WASM
// instructions, not native stubs that could bypass the runtime under test.
func tinyReactor(apiVersion byte, initialization []byte) []byte {
	module := []byte{0, 'a', 's', 'm', 1, 0, 0, 0}
	appendSection := func(id byte, payload []byte) {
		module = append(module, id, byte(len(payload)))
		module = append(module, payload...)
	}
	appendSection(1, []byte{4, 0x60, 0, 0, 0x60, 0, 1, 0x7f, 0x60, 1, 0x7f, 1, 0x7f, 0x60, 2, 0x7f, 0x7f, 1, 0x7e})
	appendSection(3, []byte{4, 0, 1, 2, 3})
	appendSection(5, []byte{1, 0, 1})
	exports := []byte{5}
	for index, name := range []string{"_initialize", "kumbuka_api_version", "kumbuka_alloc", "kumbuka_transform", "memory"} {
		kind, address := byte(0), byte(index)
		if name == "memory" {
			kind, address = 2, 0
		}
		exports = append(exports, byte(len(name)))
		exports = append(exports, name...)
		exports = append(exports, kind, address)
	}
	appendSection(7, exports)
	code := []byte{4}
	for _, instructions := range [][]byte{initialization, {0x41, apiVersion}, {0x41, 16}, {0x42, 0}} {
		body := append([]byte{0}, instructions...)
		body = append(body, 0x0b)
		code = append(code, byte(len(body)))
		code = append(code, body...)
	}
	appendSection(10, code)
	return module
}

// TestRuntimeChecksGuestVersionAndInitializationDeadline verifies runtime checks guest version and initialization deadline behavior.
func TestRuntimeChecksGuestVersionAndInitializationDeadline(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		version  byte
		body     []byte
		expected string
	}{
		{"version", 2, nil, "API version"},
		{"initialization trap", 1, []byte{0}, "initialize"},
		{"initialization loop", 1, []byte{0x03, 0x40, 0x0c, 0, 0x0b}, "deadline"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			runtime, err := wasm.New(context.Background(), wasm.Limits{CallTimeout: 50 * time.Millisecond})
			require.NoError(t, err)
			defer func() { _ = runtime.Close(context.Background()) }()
			pkg, err := pluginpackage.Read(archive(t, tinyReactor(scenario.version, scenario.body), "preprocess"))
			require.NoError(t, err)
			_, err = runtime.Load(context.Background(), pkg)
			require.ErrorContains(t, err, scenario.expected)
		})
	}
}

// TestRuntimeRejectsForeignImports verifies runtime rejects foreign imports behavior.
func TestRuntimeRejectsForeignImports(t *testing.T) {
	// (module (type (func)) (import "evil" "f" (func (type 0))))
	module := []byte{0, 'a', 's', 'm', 1, 0, 0, 0, 1, 4, 1, 0x60, 0, 0, 2, 10, 1, 4, 'e', 'v', 'i', 'l', 1, 'f', 0, 0}
	runtime, err := wasm.New(context.Background(), wasm.Limits{})
	require.NoError(t, err)
	defer func() { _ = runtime.Close(context.Background()) }()
	pkg, err := pluginpackage.Read(archive(t, module, "preprocess"))
	require.NoError(t, err)
	_, err = runtime.Load(context.Background(), pkg)
	require.ErrorContains(t, err, "unsupported WASM import evil.f")
}
