package wasm

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCertificateRootLoaderRejectsFileBeyondRemainingBudget(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "roots.pem")
	require.NoError(t, os.WriteFile(path, []byte("12345"), 0o600))
	loader := certificateRootLoader{roots: x509.NewCertPool(), budget: 4}

	added, err := loader.addFile(path)

	require.ErrorIs(t, err, errHTTPUnavailable)
	require.False(t, added)
	require.Equal(t, int64(4), loader.budget)
}

func TestCertificateRootLoaderRejectsDirectoryWithoutCertificates(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "not-a-certificate.pem"), []byte("not a certificate"), 0o600))
	loader := certificateRootLoader{roots: x509.NewCertPool(), budget: 1024}

	err := loader.addDirectory(directory)

	require.ErrorIs(t, err, errHTTPUnavailable)
}
