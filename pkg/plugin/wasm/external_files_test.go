package wasm_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	sdk "github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/require"
)

// TestExternalFilesPackage exercises the companion plugin's real WASM through
// registration, capability dispatch, Markdown expansion and final sanitization.
func TestExternalFilesPackage(t *testing.T) {
	path := os.Getenv("KUMBUKA_EXTERNAL_FILES_TEST_PACKAGE")
	if path == "" {
		t.Skip("set KUMBUKA_EXTERNAL_FILES_TEST_PACKAGE to the companion built package")
	}
	archive, err := os.ReadFile(path)
	require.NoError(t, err)
	ctx := context.Background()
	calls := 0
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithPermissions("external:read"), wasm.WithExternalFiles(func(_ context.Context, raw json.RawMessage) (any, error) {
		calls++
		var q sdk.ExternalFileRequest
		require.NoError(t, json.Unmarshal(raw, &q))
		require.Equal(t, 2, q.Start)
		require.Equal(t, 3, q.End)
		return sdk.ExternalFile{Start: 2, Content: "<script>alert(1)</script>\n{{external-file source=\"nested\" path=\"secret\"}}"}, nil
	}))
	require.NoError(t, err)
	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime)
	t.Cleanup(func() { _ = manager.Close(ctx) })
	_, err = manager.Install(ctx, archive)
	require.NoError(t, err)
	require.NoError(t, manager.Enable(ctx, "me.kumbuka.external-files"))
	renderer := markdown.NewWithRegistry(registry)
	source := `{{external-file source="docs" path="example.go" lines="2-3" note="3:Describe <unsafe> content."}}`
	got, err := renderer.RenderPageResolvedWithFunctions(source, markdown.Slug, markdown.DefaultOptions(), markdown.Functions{Context: ctx})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Contains(t, got.HTML, "external-file-number")
	require.Contains(t, got.HTML, "Line 3:")
	require.Contains(t, got.HTML, "[1]")
	require.NotContains(t, got.HTML, "<script>")
	require.Contains(t, got.HTML, "&lt;script&gt;")
	require.Contains(t, got.HTML, "&lt;unsafe&gt;")
	require.False(t, renderer.CanPersist(source, nil))
	_, err = renderer.RenderPageResolvedWithFunctions("```\n"+source+"\n```", markdown.Slug, markdown.DefaultOptions(), markdown.Functions{Context: ctx})
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	if output := os.Getenv("KUMBUKA_EXTERNAL_FILES_TEST_HTML"); output != "" {
		require.NoError(t, os.WriteFile(output, []byte(strings.TrimSpace(got.HTML)), 0600))
	}
}
