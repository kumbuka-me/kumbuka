package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lifecycleTestRuntime returns isolated no-op instances for manager lifecycle tests.
type lifecycleTestRuntime struct{}

// Load creates one no-op plugin instance.
func (lifecycleTestRuntime) Load(context.Context, *pluginpackage.Package) (Instance, error) {
	return lifecycleTestInstance{}, nil
}

// Close releases the no-op runtime.
func (lifecycleTestRuntime) Close(context.Context) error { return nil }

// lifecycleTestInstance supplies an empty contribution set.
type lifecycleTestInstance struct{}

// Contributions returns no executable contributions.
func (lifecycleTestInstance) Contributions() Contributions { return Contributions{} }

// Close releases the no-op instance.
func (lifecycleTestInstance) Close(context.Context) error { return nil }

func TestUninstallDoesNotRestoreBuiltinFallback(t *testing.T) {
	ctx := context.Background()
	const id = "io.example.lifecycle"
	manager := NewManager(&Registry{}, lifecycleTestRuntime{})
	t.Cleanup(func() { require.NoError(t, manager.Close(ctx)) })
	require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{lifecycleTestArchive(t, id, "1.0.0")})))
	_, err := manager.Upgrade(ctx, id, lifecycleTestArchive(t, id, "2.0.0"))
	require.NoError(t, err)
	require.NoError(t, manager.Uninstall(ctx, id))
	assert.Empty(t, manager.Plugins())
	records, err := manager.store.ListPlugins(ctx)
	require.NoError(t, err)
	assert.Empty(t, records)
}

// lifecycleTestArchive creates one valid declarative package with the requested version.
func lifecycleTestArchive(t *testing.T, id, version string) []byte {
	t.Helper()
	return lifecycleTestArchiveWithREADME(t, id, version, "# Lifecycle fixture\n")
}

// lifecycleTestArchiveWithREADME creates a valid package with caller-selected documentation bytes.
func lifecycleTestArchiveWithREADME(t *testing.T, id, version, readme string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entries := map[string]string{
		"README.md": readme,
		"plugin.yaml": fmt.Sprintf(
			"api_version: 1\nid: %s\nname: Lifecycle fixture\nversion: %s\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\npermissions: []\n",
			id,
			version,
		),
	}

	for name, data := range entries {
		file, err := writer.Create(name)
		require.NoError(t, err)
		_, err = file.Write([]byte(data))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	return buffer.Bytes()
}
