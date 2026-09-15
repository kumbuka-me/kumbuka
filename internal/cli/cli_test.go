package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunShowsRootHelpWhenCommandIsMissing(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	err := Run(context.Background(), nil, nil, "test", "deadbeef", io.Discard, &stderr)

	require.NoError(t, err)
	assert.Contains(t, stderr.String(), "serve")
	assert.Contains(t, stderr.String(), "build")
	assert.Contains(t, stderr.String(), "plugins")
}

func TestRunShowsServeHelpWithoutRuntimeConfiguration(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), []string{"serve", "--help"}, nil, "test", "deadbeef", &stdout, io.Discard)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "--database-url")
	assert.Contains(t, stdout.String(), "--listen-address")
}

func TestRunShowsBuildHelp(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := Run(context.Background(), []string{"build", "--help"}, nil, "test", "deadbeef", &stdout, io.Discard)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "--config")
	assert.Contains(t, stdout.String(), "--site-name")
	assert.Contains(t, stdout.String(), "--plugins")
}

func TestRunPrintsConfigurationErrors(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), []string{
		"serve",
		"--database-url", "postgres://example/kumbuka",
		"--encryption-key", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}, nil, "test", "deadbeef", &stdout, &stderr)

	require.ErrorContains(t, err, "encryption key must be a base64-encoded 32-byte value")
	assert.Equal(t, err.Error()+"\n", stderr.String())
	assert.Empty(t, stdout.String())
}

func TestRunListsProjectPlugins(t *testing.T) {
	t.Parallel()

	filename := filepath.Join(t.TempDir(), ".kumbukaplugins")
	require.NoError(t, os.WriteFile(filename, []byte(`format = 1

[[plugin]]
id = "me.kumbuka.mermaid"
repository = "kumbuka-me/plugins"
tag_prefix = "mermaid/v"
asset = "mermaid"
version = "1.0.1"
`), 0o644))

	var stdout bytes.Buffer
	err := Run(context.Background(), []string{"plugins", "list", "--file", filename}, nil, "test", "deadbeef", &stdout, io.Discard)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "me.kumbuka.mermaid 1.0.1")
	assert.Contains(t, stdout.String(), "kumbuka-me/plugins@mermaid/v1.0.1")
}
