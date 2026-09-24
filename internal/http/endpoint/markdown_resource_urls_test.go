package endpoint

import (
	"bytes"
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/application/portablearchive"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportedMarkdownOnlyRewritesResourceDestinations(t *testing.T) {
	media := &exportMediaStub{}
	source := "External ![image](https://example.org/media/12/external.png)\n" +
		"Literal /media/12/example.png and `![code](/media/12/code.png)`\n" +
		"```md\n![fenced](/media/12/fenced.png)\n```\n" +
		"![local](/media/12/local.png)\n[reference]: /media/12/reference.png\n" +
		"<img src=\"/media/12/html.png\" alt=\"/media/12/alt.png\">\n"
	want := "External ![image](https://example.org/media/12/external.png)\n" +
		"Literal /media/12/example.png and `![code](/media/12/code.png)`\n" +
		"```md\n![fenced](/media/12/fenced.png)\n```\n" +
		"![local](../media/12/image.png)\n[reference]: ../media/12/image.png\n" +
		"<img src=\"../media/12/image.png\" alt=\"/media/12/alt.png\">\n"

	got, ids, err := portablearchive.ExportedMarkdown(context.Background(), media, "pages/start.md", source, map[int64]domain.ImageData{})
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, []int64{12}, ids)
	assert.Equal(t, ids, portablearchive.ReferencedImageIDs(source))
	assert.Equal(t, []int64{12}, media.calls)
}

func TestPortableExportOnlyRewritesResourceDestinations(t *testing.T) {
	var archive bytes.Buffer
	source := "[external](https://example.org/media/99/remote.png) " +
		"`[code](/media/99/code.png)` /media/99/example.png " +
		"[local](/media/12/diagram.png)"
	err := portablearchive.WritePortable(context.Background(), portableCatalogStub{"example": {
		Slug: "example", Markdown: source,
	}}, portableExportMediaStub{}, &archive, []string{"example"})
	require.NoError(t, err)
	files := readTestZip(t, archive.Bytes())
	got := string(files["pages/example.md"])
	assert.Equal(t, "[external](https://example.org/media/99/remote.png) "+
		"`[code](/media/99/code.png)` /media/99/example.png "+
		"[local](../media/12/diagram.png)", got)
	assert.Contains(t, files, "media/12/diagram.png")
}

func TestPortableImportOnlyRewritesResourceDestinations(t *testing.T) {
	source := "[local](../../media/12/diagram.png) " +
		"[external](https://example.org/media/12/diagram.png) " +
		"`[code](../../media/12/diagram.png)` " +
		"../../media/12/diagram.png\n" +
		"[reference]: ../../media/12/diagram.png\n" +
		"<img src='../../media/12/diagram.png' alt='../../media/12/diagram.png'>"
	want := "[local](/media/101/diagram.png) " +
		"[external](https://example.org/media/12/diagram.png) " +
		"`[code](../../media/12/diagram.png)` " +
		"../../media/12/diagram.png\n" +
		"[reference]: /media/101/diagram.png\n" +
		"<img src='/media/101/diagram.png' alt='../../media/12/diagram.png'>"

	got, err := portablearchive.RestoreResourceReferences("pages/team/example.md", source, map[string]string{
		"media/12/diagram.png": "/media/101/diagram.png",
	})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
