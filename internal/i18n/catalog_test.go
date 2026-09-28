package i18n

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogResolutionAndFallback(t *testing.T) {
	t.Parallel()

	catalog, err := Load(fstest.MapFS{
		"locales/en.toml": &fstest.MapFile{Data: []byte(`language = "en"
label = "English"
[messages]
"hello" = "Hello"
"only_english" = "Fallback"
"browser.close" = "Close"
`)},
		"locales/de.toml": &fstest.MapFile{Data: []byte(`language = "de"
label = "Deutsch"
[messages]
"hello" = "Hallo"
"browser.close" = "Schliessen"
`)},
	})
	require.NoError(t, err)

	german := catalog.Resolve("", "de-CH,de;q=0.9,en;q=0.8")
	assert.Equal(t, "de", german.Code)
	assert.Equal(t, "Hallo", german.Text("hello"))
	assert.Equal(t, "Fallback", german.Text("only_english"))
	assert.Equal(t, "missing", german.Text("missing"))
	assert.Equal(t, map[string]string{"browser.close": "Schliessen"}, german.BrowserMessages())

	explicit := catalog.Resolve("en", "de")
	assert.Equal(t, "en", explicit.Code)

	unsupported := catalog.Resolve("", "fr-FR,fr;q=0.9")
	assert.Equal(t, "en", unsupported.Code)
}

func TestCatalogRequiresEnglish(t *testing.T) {
	t.Parallel()

	_, err := Load(fstest.MapFS{
		"locales/de.toml": &fstest.MapFile{Data: []byte(`language = "de"
label = "Deutsch"
[messages]
"hello" = "Hallo"
`)},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "default locale catalog")
}
