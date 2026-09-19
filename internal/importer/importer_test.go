package importer

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFormatRequiresExplicitFormat(t *testing.T) {
	t.Parallel()

	t.Run("empty format", func(t *testing.T) {
		t.Parallel()

		_, err := ParseFormat("")
		assert.Error(t, err)
	})

	t.Run("automatic format", func(t *testing.T) {
		t.Parallel()

		_, err := ParseFormat("auto")
		assert.Error(t, err)
	})

	t.Run("ambiguous JSON format", func(t *testing.T) {
		t.Parallel()

		_, err := ParseFormat("json")
		assert.Error(t, err)
	})

	t.Run("explicit Wiki.js format", func(t *testing.T) {
		t.Parallel()

		format, err := ParseFormat("wikijs")
		require.NoError(t, err)
		assert.Equal(t, FormatWikiJS, format)
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

func TestParseFileUsesSharedBudget(t *testing.T) {
	t.Parallel()

	budget := NewBudget(10)
	items, err := ParseFile("first.md", bytes.NewBufferString("# Title"), FormatMarkdown, budget)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, int64(3), budget.Remaining())

	_, err = ParseFile("second.md", bytes.NewBufferString("# Other"), FormatMarkdown, budget)
	assert.Error(t, err)
	assert.Equal(t, "Import contents exceed 100 MiB.", importerUserMessage(t, err))
}

func TestImportZIPBudgetSharedAcrossFiles(t *testing.T) {
	t.Parallel()

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("page.md")
	require.NoError(t, err)
	_, err = entry.Write([]byte("# Title"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	budget := NewBudget(10)
	items, err := importZIP(archive.Bytes(), FormatMarkdown, budget)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, int64(3), budget.Remaining())

	_, err = importZIP(archive.Bytes(), FormatMarkdown, budget)
	require.Error(t, err)
	assert.Equal(t, "Archive contents exceed 100 MiB.", importerUserMessage(t, err))
}

func TestHTMLToMarkdownSeparatesBlockAndInlineFormatting(t *testing.T) {
	t.Parallel()

	markdown, err := htmlToMarkdown([]byte(`<h1>Guide</h1><p>Hello <strong>world</strong>.</p><pre><code>echo ok</code></pre>`))

	require.NoError(t, err)
	assert.Contains(t, markdown, "# Guide")
	assert.Contains(t, markdown, "Hello **world**.")
	assert.Contains(t, markdown, "```\necho ok\n```")
}

func importerUserMessage(t *testing.T, err error) string {
	t.Helper()

	type userMessageError interface {
		error
		UserMessage() string
	}

	var userErr userMessageError
	require.True(t, errors.As(err, &userErr))
	return userErr.UserMessage()
}
