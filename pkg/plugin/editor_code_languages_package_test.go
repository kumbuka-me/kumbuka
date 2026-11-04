package plugin

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/require"
)

// TestEditorCodeLanguagesFromPackage checks SDK contract parsing from actual plugin archives.
func TestEditorCodeLanguagesFromPackage(t *testing.T) {
	t.Parallel()

	t.Run("valid declarations", func(t *testing.T) {
		t.Parallel()
		pkg := codeLanguageTestPackage(t, `{"version":1,"languages":[{"value":"runbook","label":"Runbook"}]}`)
		languages, problem := editorCodeLanguagesFromPackage(pkg)
		require.Empty(t, problem)
		require.Equal(t, []EditorCodeLanguage{{PluginID: "me.kumbuka.test-languages", Value: "runbook", Label: "Runbook"}}, languages)
	})

	t.Run("invalid declarations", func(t *testing.T) {
		t.Parallel()
		pkg := codeLanguageTestPackage(t, `{"version":1,"languages":[{"value":"Runbook","label":"Runbook"}]}`)
		languages, problem := editorCodeLanguagesFromPackage(pkg)
		require.Empty(t, languages)
		require.NotEmpty(t, problem)
	})
}

// codeLanguageTestPackage builds a tiny declarative plugin with language declarations.
func codeLanguageTestPackage(t *testing.T, languages string) *pluginpackage.Package {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	entries := map[string]string{
		"README.md":                  "# Test code languages\n",
		"plugin.yaml":                "api_version: 1\nid: me.kumbuka.test-languages\nname: Test languages\nversion: 1.0.0\ndefault_enabled: true\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\npermissions: []\n",
		"assets/code-languages.json": languages,
	}
	for path, content := range entries {
		file, err := archive.Create(path)
		require.NoError(t, err)
		_, err = file.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	pkg, err := pluginpackage.Read(buffer.Bytes())
	require.NoError(t, err)
	return pkg
}
