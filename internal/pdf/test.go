package pdf

import (
	"bytes"
	"compress/zlib"
	"context"
	_ "embed"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"strings"
)

//go:embed testfixture/document.html
var testDocumentHTML string

//go:embed testfixture/image.png
var testImage []byte

// TestResult describes the generated PDF returned by a PDF service test.
type TestResult struct {
	File      *os.File
	PageCount int
	Size      int64
}

// RenderTest renders Kumbuka's fixed PDF diagnostic document and returns it for inspection.
func RenderTest(ctx context.Context, endpoint string, headers http.Header) (result TestResult, cleanup func(), err error) {
	file, cleanup, err := Render(ctx, endpoint, "Kumbuka PDF service test", "en", testBody(), headers)
	if err != nil {
		return TestResult{}, cleanup, err
	}

	info, err := file.Stat()
	if err != nil {
		cleanup()
		return TestResult{}, func() {}, err
	}

	pageCount, err := pdfPageCount(file)
	if err != nil {
		pageCount = 0
	}

	return TestResult{
		File:      file,
		PageCount: pageCount,
		Size:      info.Size(),
	}, cleanup, nil
}

// testBody returns a two-page self-contained document that exercises common export features.
func testBody() string {
	image := base64.StdEncoding.EncodeToString(testImage)

	return strings.Replace(testDocumentHTML, "{{TEST_IMAGE_BASE64}}", image, 1)
}

// pdfPageCount counts page objects in plain or Flate-compressed PDF object streams.
func pdfPageCount(file *os.File) (int, error) {
	position, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	defer func() { _, _ = file.Seek(position, io.SeekStart) }()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	data, err := io.ReadAll(io.LimitReader(file, maxPDFBytes+1))
	if err != nil {
		return 0, err
	}

	count := countPDFPageObjects(withoutPDFStreamBodies(data))
	for _, decoded := range decodedFlateStreams(data) {
		count += countPDFPageObjects(decoded)
	}

	return count, nil
}

// withoutPDFStreamBodies removes raw stream payloads before scanning the PDF body.
// Compressed object streams may contain literal /Type /Page bytes by chance, and
// those objects are counted separately after the stream has been decoded.
func withoutPDFStreamBodies(data []byte) []byte {
	stripped := bytes.Clone(data)
	offset := 0

	for offset < len(data) {
		relative := bytes.Index(data[offset:], []byte("stream"))
		if relative < 0 {
			break
		}

		stream := offset + relative
		start := stream + len("stream")
		if start < len(data) && data[start] == '\r' {
			start++
		}
		if start >= len(data) || data[start] != '\n' {
			offset = start
			continue
		}
		start++

		relativeEnd := bytes.Index(data[start:], []byte("endstream"))
		if relativeEnd < 0 {
			break
		}

		end := start + relativeEnd
		for index := start; index < end; index++ {
			stripped[index] = ' '
		}

		offset = end + len("endstream")
	}

	return stripped
}

// countPDFPageObjects counts /Type /Page dictionaries while excluding /Pages nodes.
func countPDFPageObjects(data []byte) int {
	count := 0
	offset := 0

	for offset < len(data) {
		index := bytes.Index(data[offset:], []byte("/Type"))
		if index < 0 {
			break
		}

		cursor := offset + index + len("/Type")
		for cursor < len(data) && isPDFWhitespace(data[cursor]) {
			cursor++
		}
		if cursor >= len(data) || data[cursor] != '/' {
			offset = cursor
			continue
		}

		cursor++
		start := cursor
		for cursor < len(data) && !isPDFDelimiter(data[cursor]) {
			cursor++
		}
		if string(data[start:cursor]) == "Page" {
			count++
		}

		offset = cursor
	}

	return count
}

// decodedFlateStreams returns streams using the common FlateDecode PDF filter.
func decodedFlateStreams(data []byte) [][]byte {
	var decoded [][]byte
	offset := 0

	for offset < len(data) {
		relative := bytes.Index(data[offset:], []byte("stream"))
		if relative < 0 {
			break
		}

		stream := offset + relative
		start := stream + len("stream")
		if start < len(data) && data[start] == '\r' {
			start++
		}
		if start >= len(data) || data[start] != '\n' {
			offset = start
			continue
		}
		start++

		relativeEnd := bytes.Index(data[start:], []byte("endstream"))
		if relativeEnd < 0 {
			break
		}
		end := start + relativeEnd
		offset = end + len("endstream")

		preambleStart := max(0, stream-512)
		if !bytes.Contains(data[preambleStart:stream], []byte("/FlateDecode")) {
			continue
		}

		reader, err := zlib.NewReader(bytes.NewReader(bytes.TrimRight(data[start:end], "\r\n")))
		if err != nil {
			continue
		}

		content, err := io.ReadAll(reader)
		_ = reader.Close()
		if err == nil {
			decoded = append(decoded, content)
		}
	}

	return decoded
}

// isPDFWhitespace reports whether a byte is a whitespace character.
func isPDFWhitespace(value byte) bool {
	switch value {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}

// isPDFDelimiter reports whether a byte is a delimiter character.
func isPDFDelimiter(value byte) bool {
	if isPDFWhitespace(value) {
		return true
	}

	switch value {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}
