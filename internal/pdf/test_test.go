package pdf

import (
	"bytes"
	"compress/zlib"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestBodyExercisesPDFRenderingFeatures(t *testing.T) {
	t.Parallel()

	body := testBody()

	assert.Contains(t, body, "data:image/png;base64,")
	assert.NotContains(t, body, "{{TEST_IMAGE_BASE64}}")
	assert.True(t, bytes.HasPrefix(testImage, []byte("\x89PNG\r\n\x1a\n")))
	assert.Contains(t, body, "$(VAR_NAME)")
	assert.Contains(t, body, "ä ö ü ß é")
	assert.Contains(t, body, "日本語")
	assert.Contains(t, body, "<table>")
	assert.Contains(t, body, "class=\"callout warning\"")
	assert.Contains(t, body, "break-before: page")
}

func TestPDFPageCount(t *testing.T) {
	t.Parallel()

	t.Run("plain page objects", func(t *testing.T) {
		t.Parallel()

		file := writePDFTestFile(t, []byte(`%PDF-1.7
1 0 obj <</Type /Pages/Kids [2 0 R 3 0 R]/Count 2>> endobj
2 0 obj <</Type /Page/Parent 1 0 R>> endobj
3 0 obj <</Type/Page/Parent 1 0 R>> endobj
%%EOF`))

		count, err := pdfPageCount(file)

		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("ignores page markers inside raw stream bodies", func(t *testing.T) {
		t.Parallel()

		file := writePDFTestFile(t, []byte(`%PDF-1.7
1 0 obj <</Type /Pages/Kids [2 0 R 3 0 R]/Count 2>> endobj
2 0 obj <</Type /Page/Parent 1 0 R>> endobj
3 0 obj <</Type /Page/Parent 1 0 R>> endobj
4 0 obj <</Length 25>>
stream
/Type /Page /Type /Page
endstream
endobj
%%EOF`))

		count, err := pdfPageCount(file)

		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})

	t.Run("flate compressed object stream", func(t *testing.T) {
		t.Parallel()

		objectStream := []byte(`1 0 2 48 3 96
<</Type /Pages/Kids [2 0 R 3 0 R]/Count 2>>
<</Type /Page/Parent 1 0 R>>
<</Type /Page/Parent 1 0 R>>`)
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)

		_, err := writer.Write(objectStream)
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		var pdf bytes.Buffer
		pdf.WriteString("%PDF-1.7\n19 0 obj\n<</Type /ObjStm/Filter /FlateDecode/Length 999>>\nstream\n")
		pdf.Write(compressed.Bytes())
		pdf.WriteString("\nendstream\nendobj\n%%EOF\n")

		file := writePDFTestFile(t, pdf.Bytes())
		count, err := pdfPageCount(file)

		require.NoError(t, err)
		assert.Equal(t, 2, count)
	})
}

func writePDFTestFile(t *testing.T, content []byte) *os.File {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "test-*.pdf")
	require.NoError(t, err)

	_, err = file.Write(content)
	require.NoError(t, err)

	_, err = file.Seek(0, 0)
	require.NoError(t, err)

	t.Cleanup(func() { _ = file.Close() })
	return file
}
