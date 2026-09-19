package portable

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testArchiveLimit = 100 << 20

// TestParseRejectsUnsupportedVersion verifies that future archive formats fail safely.
func TestParseRejectsUnsupportedVersion(t *testing.T) {
	t.Parallel()

	manifest := NewManifest()
	manifest.Version++
	manifest.Pages = []PageEntry{{
		Slug: "guide", Markdown: "pages/guide.md", Metadata: "metadata/guide.json",
	}}
	archive := buildTestArchive(t, manifest, map[string][]byte{
		"pages/guide.md": []byte("# Guide\n"),
		"metadata/guide.json": mustTestJSON(t, PageMetadata{
			Slug: "guide", Title: "Guide", Status: "verified",
		}),
	})

	_, err := Parse(archive, testArchiveLimit)

	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
	assert.Contains(t, validation.Message, "not supported")
}

// TestParseRejectsUnsafeAndUnlistedPaths verifies archive inventory and traversal validation.
func TestParseRejectsUnsafeAndUnlistedPaths(t *testing.T) {
	t.Parallel()

	manifest := NewManifest()
	manifest.Pages = []PageEntry{{
		Slug: "guide", Markdown: "pages/guide.md", Metadata: "metadata/guide.json",
	}}
	base := map[string][]byte{
		"pages/guide.md": []byte("# Guide\n"),
		"metadata/guide.json": mustTestJSON(t, PageMetadata{
			Slug: "guide", Title: "Guide", Status: "verified",
		}),
	}

	t.Run("traversal", func(t *testing.T) {
		files := cloneTestFiles(base)
		files["../escape.txt"] = []byte("escape")
		_, err := Parse(buildTestArchive(t, manifest, files), testArchiveLimit)

		var validation *ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Contains(t, validation.Message, "unsafe file path")
	})

	t.Run("unlisted", func(t *testing.T) {
		files := cloneTestFiles(base)
		files["extra.txt"] = []byte("extra")
		_, err := Parse(buildTestArchive(t, manifest, files), testArchiveLimit)

		var validation *ValidationError
		require.ErrorAs(t, err, &validation)
		assert.Contains(t, validation.Message, "not listed")
	})
}

// TestDetectRecognizesPortableManifest verifies cheap upload auto-detection.
func TestDetectRecognizesPortableManifest(t *testing.T) {
	t.Parallel()

	manifest := NewManifest()
	data := buildTestArchive(t, manifest, nil)

	assert.True(t, Detect(data, testArchiveLimit))
	assert.False(t, Detect([]byte("not a zip"), testArchiveLimit))
}

// buildTestArchive constructs one ZIP containing manifest plus supplied files.
func buildTestArchive(t *testing.T, manifest Manifest, files map[string][]byte) []byte {
	t.Helper()

	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	manifestData := mustTestJSON(t, manifest)
	writer, err := archive.Create(ManifestPath)
	require.NoError(t, err)
	_, err = writer.Write(manifestData)
	require.NoError(t, err)
	for name, data := range files {
		writer, err := archive.Create(name)
		require.NoError(t, err)
		_, err = writer.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	return output.Bytes()
}

// mustTestJSON marshals a portable test value or fails the test.
func mustTestJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

// cloneTestFiles copies a test archive file map before adding case-specific entries.
func cloneTestFiles(source map[string][]byte) map[string][]byte {
	result := make(map[string][]byte, len(source)+1)
	for name, data := range source {
		result[name] = bytes.Clone(data)
	}
	return result
}

func TestDetectLargeManifestWithinImportBudget(t *testing.T) {
	t.Parallel()
	manifest := NewManifest()
	for i := range 1000 {
		slug := fmt.Sprintf("guide-%d", i)
		manifest.Pages = append(manifest.Pages, PageEntry{Slug: slug, Markdown: "pages/" + slug + ".md", Metadata: "metadata/" + slug + ".json"})
	}
	manifestSize := int64(len(mustTestJSON(t, manifest)))
	require.Greater(t, manifestSize, int64(64<<10))
	data := buildTestArchive(t, manifest, nil)
	assert.True(t, Detect(data, manifestSize))
	assert.False(t, Detect(data, manifestSize-1))
	assert.False(t, Detect(data, 0))
}
