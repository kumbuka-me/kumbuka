package handler

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseImportFormatRequiresExplicitFormat(t *testing.T) {
	t.Parallel()

	t.Run("empty format", func(t *testing.T) {
		t.Parallel()

		_, err := parseImportFormat("")
		assert.Error(t, err)
	})

	t.Run("automatic format", func(t *testing.T) {
		t.Parallel()

		_, err := parseImportFormat("auto")
		assert.Error(t, err)
	})

	t.Run("ambiguous JSON format", func(t *testing.T) {
		t.Parallel()

		_, err := parseImportFormat("json")
		assert.Error(t, err)
	})

	t.Run("explicit Wiki.js format", func(t *testing.T) {
		t.Parallel()

		format, err := parseImportFormat("wikijs")
		require.NoError(t, err)
		assert.Equal(t, wikiJSImport, format)
	})
}

func TestImportWikiJSONRequiresExplicitPageFields(t *testing.T) {
	t.Parallel()

	t.Run("explicit page fields", func(t *testing.T) {
		t.Parallel()

		pages, err := importWikiJSON([]byte(`[{"path":"guides/setup","title":"Setup","content":"# Setup\n"}]`))
		require.NoError(t, err)
		require.Len(t, pages, 1)
		assert.Equal(t, "guides/setup", pages[0].Slug)
		assert.Equal(t, "Setup", pages[0].Title)
	})

	t.Run("nested pages object", func(t *testing.T) {
		t.Parallel()

		_, err := importWikiJSON([]byte(`{"pages":[{"path":"guide","title":"Guide","content":"# Guide"}]}`))
		assert.Error(t, err)
	})

	t.Run("missing path", func(t *testing.T) {
		t.Parallel()

		_, err := importWikiJSON([]byte(`[{"title":"Guide","content":"# Guide"}]`))
		assert.Error(t, err)
	})

	t.Run("missing title", func(t *testing.T) {
		t.Parallel()

		_, err := importWikiJSON([]byte(`[{"path":"guide","content":"# Guide"}]`))
		assert.Error(t, err)
	})

	t.Run("missing content", func(t *testing.T) {
		t.Parallel()

		_, err := importWikiJSON([]byte(`[{"path":"guide","title":"Guide"}]`))
		assert.Error(t, err)
	})
}

func TestMarkdownTitleRequiresExplicitHeading(t *testing.T) {
	t.Parallel()

	t.Run("explicit heading", func(t *testing.T) {
		t.Parallel()

		title, err := markdownTitle("intro\n# Explicit title\n")
		require.NoError(t, err)
		assert.Equal(t, "Explicit title", title)
	})

	t.Run("missing heading", func(t *testing.T) {
		t.Parallel()

		_, err := markdownTitle("No title here")
		assert.Error(t, err)
	})
}

func TestReadImportArchiveEntryEnforcesRemainingBudget(t *testing.T) {
	t.Parallel()

	_, err := readImportArchiveEntry(io.NopCloser(strings.NewReader("abc")), 2)

	require.EqualError(t, err, "archive contents exceed 100 MiB")
}

func TestImportZIPBudgetSharedAcrossFiles(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("page.md")
	require.NoError(t, err)
	_, err = entry.Write([]byte("# Title"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	remaining := int64(10)
	items, err := importZIP(archive.Bytes(), markdownImport, &remaining)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(3), remaining)
	_, err = importZIP(archive.Bytes(), markdownImport, &remaining)
	require.ErrorContains(t, err, "archive contents exceed 100 MiB")

	message, ok := userErrorMessage(err)
	require.True(t, ok)
	assert.Equal(t, "Archive contents exceed 100 MiB.", message)
}
