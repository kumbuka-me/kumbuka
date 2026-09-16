package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoreDoesNotDependOnFirstPartyPluginIDs(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))

	for _, directory := range []string{"cmd", "internal", "pkg", filepath.Join("web", "src")} {
		err := filepath.WalkDir(filepath.Join(root, directory), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".gohtml", ".ts", ".js", ".css":
			default:
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(content), "me.kumbuka.") || strings.Contains(string(content), "io.lore.") {
				t.Errorf("generic core source must not depend on a first-party plugin ID: %s", path)
			}
			return nil
		})
		require.NoError(t, err)
	}
}
