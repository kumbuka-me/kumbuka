package endpoint

import (
	"context"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownExportPreservesImageWidths(t *testing.T) {
	t.Parallel()

	media := &exportMediaStub{}
	source := "![Diagram](/media/12/old.png){width=50%}\n\n" +
		"![Diagram][image]{width=640px}\n\n[image]: /media/12/old.png \"Overview\"\n"
	got, ids, err := exportedMarkdown(context.Background(), media, "pages/start.md", source, map[int64]domain.ImageData{})

	require.NoError(t, err)
	assert.Equal(t, strings.ReplaceAll(source, "/media/12/old.png", "../media/12/image.png"), got)
	assert.Equal(t, []int64{12}, ids)
	assert.Equal(t, ids, referencedImageIDs(source))
	assert.Equal(t, ids, media.calls)
}

func TestPDFMediaInliningPreservesRenderedImageWidths(t *testing.T) {
	t.Parallel()

	media := &exportMediaStub{}
	rendered, err := testMarkdownRenderer(t).Render(`![Diagram](/media/12/image.png){width=50%}`)
	require.NoError(t, err)

	got, err := inlineRenderedMedia(context.Background(), media, rendered)

	require.NoError(t, err)
	assert.Contains(t, got, `src="data:image/png;base64,aW1hZ2U="`)
	assert.Contains(t, strings.ReplaceAll(got, " ", ""), `style="width:50%"`)
	assert.NotContains(t, got, "{width=")
	assert.Equal(t, []int64{12}, media.calls)
}
