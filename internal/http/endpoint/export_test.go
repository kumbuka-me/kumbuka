package endpoint

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exportMediaStub provides controllable export media behavior for tests.
type exportMediaStub struct {
	// calls counts calls observed by the test double.
	calls []int64
	// err configures the error returned by the test double.
	err error
}

func (s *exportMediaStub) ImageContent(_ context.Context, id int64) (domain.ImageData, error) {
	s.calls = append(s.calls, id)
	return domain.ImageData{Filename: "image.png", ContentType: "image/png", Data: []byte("image")}, s.err
}

func TestExportedMarkdownScansMediaReferences(t *testing.T) {
	media := &exportMediaStub{}
	source := `![a](/media/12/old.png) ![b](/media/12/old.png) /media/no/file /media/4/ /media/999999999999999999999/x [image](/media/7/image.png)`
	got, ids, err := exportedMarkdown(context.Background(), media, "pages/start.md", source, map[int64]domain.ImageData{})
	require.NoError(t, err)
	assert.Equal(t, `![a](../media/12/image.png) ![b](../media/12/image.png) /media/no/file /media/4/ /media/999999999999999999999/x [image](../media/7/image.png)`, got)
	assert.Equal(t, []int64{12, 7}, ids)
	assert.Equal(t, ids, referencedImageIDs(source))
	assert.Equal(t, ids, media.calls)
}

func TestExportedMarkdownReturnsLookupError(t *testing.T) {
	failure := errors.New("lookup failed")
	media := &exportMediaStub{err: failure}
	_, _, err := exportedMarkdown(context.Background(), media, "start.md", "![one](/media/1/a) ![two](/media/2/b)", map[int64]domain.ImageData{})
	require.ErrorIs(t, err, failure)
	assert.Equal(t, []int64{1}, media.calls)
}

func TestInlineRenderedMediaUsesHTMLAttributes(t *testing.T) {
	media := &exportMediaStub{}
	source := `<p>/media/1/plain.png</p><!-- /media/1/comment.png --><img alt="/media/1/alt.png" src='/media/1/image.png'><a href="/media/1/image.png">image</a><img src="https://other.example/media/2/image.png"><img src="//other.example/media/2/image.png"><span title='A &amp; B'>text</span>`
	got, err := inlineRenderedMedia(context.Background(), media, source)
	require.NoError(t, err)
	assert.Equal(t, `<p>/media/1/plain.png</p><!-- /media/1/comment.png --><img alt="/media/1/alt.png" src="data:image/png;base64,aW1hZ2U="><a href="data:image/png;base64,aW1hZ2U=">image</a><img src="https://other.example/media/2/image.png"><img src="//other.example/media/2/image.png"><span title='A &amp; B'>text</span>`, got)
	assert.Equal(t, []int64{1}, media.calls)
}

func TestInlineRenderedMediaHandlesEscapedAndUnquotedURLs(t *testing.T) {
	media := &exportMediaStub{}
	got, err := inlineRenderedMedia(context.Background(), media, `<img src=/media/1/a.png><img src="/media/2/a%20b.png?x=1&amp;y=2"/><img src="/media/no/a"><img src="/media/3/">`)
	require.NoError(t, err)
	assert.Contains(t, got, `src="data:image/png;base64,aW1hZ2U="`)
	assert.Contains(t, got, `<img src="/media/no/a"><img src="/media/3/">`)
	assert.Equal(t, []int64{1, 2}, media.calls)
}

func TestInlineRenderedMediaReturnsLookupError(t *testing.T) {
	failure := errors.New("lookup failed")
	media := &exportMediaStub{err: failure}
	_, err := inlineRenderedMedia(context.Background(), media, `<img src="/media/1/a"><img src="/media/2/b">`)
	require.ErrorIs(t, err, failure)
	assert.Equal(t, []int64{1}, media.calls)
}

func TestExportMissingImageKeepsResourceContext(t *testing.T) {
	t.Parallel()
	_, err := inlineRenderedMedia(context.Background(), &exportMediaStub{err: domain.ErrNotFound}, `<img src="/media/1/a.png">`)
	require.ErrorIs(t, err, domain.ErrNotFound)
	response := httptest.NewRecorder()
	writeExportProblem(slog.New(slog.NewTextHandler(io.Discard, nil)), response, err)
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Contains(t, response.Body.String(), "An image referenced by this export was not found.")
	assert.NotContains(t, response.Body.String(), "Page not found")
}
