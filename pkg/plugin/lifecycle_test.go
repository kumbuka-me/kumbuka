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

func TestUninstallValidatesBundledFallbackBeforeCommit(t *testing.T) {
	t.Parallel()

	const id = "io.example.lifecycle"
	ctx := context.Background()
	bundled := lifecycleTestArchive(t, id, "1.0.0")
	installed := lifecycleTestArchive(t, id, "2.0.0")
	manager := NewManager(&Registry{}, lifecycleTestRuntime{})
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })

	require.NoError(t, manager.Bootstrap(ctx, [][]byte{bundled}))
	_, err := manager.Upgrade(ctx, id, installed)
	require.NoError(t, err)
	require.Len(t, manager.Plugins(), 1)
	assert.Equal(t, SourceInstalled, manager.Plugins()[0].Source)

	records, err := manager.store.ListPlugins(ctx)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, SourceInstalled, records[0].Source)
	require.Equal(t, installed, records[0].Package)

	// Simulate an embedded package becoming unreadable. Uninstall must fail
	// before it changes durable or in-memory lifecycle state.
	manager.mu.Lock()
	manager.bundled[id] = []byte("not a plugin archive")
	manager.mu.Unlock()

	err = manager.Uninstall(ctx, id)
	require.Error(t, err)

	plugins := manager.Plugins()
	require.Len(t, plugins, 1)
	assert.Equal(t, SourceInstalled, plugins[0].Source)
	assert.Equal(t, "2.0.0", fmt.Sprint(plugins[0].Manifest.Version))

	records, storeErr := manager.store.ListPlugins(ctx)
	require.NoError(t, storeErr)
	require.Len(t, records, 1)
	assert.Equal(t, SourceInstalled, records[0].Source)
	assert.Equal(t, installed, records[0].Package)
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
