package markdownurl

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRangesFindsMarkdownAndHTMLDestinationsOutsideCode(t *testing.T) {
	t.Parallel()

	source := strings.Join([]string{
		"[inline](one.png)",
		"[reference]: <two.png>",
		`<img alt="three" src='three.png'>`,
		`<a href=four.html>four</a>`,
		"`[code](ignored.png)`",
		"```",
		"![fenced](ignored.png)",
		"```",
	}, "\n")

	assert.Equal(t, []string{"one.png", "two.png", "three.png", "four.html"}, destinations(source))
}

func TestRangesContinuesAfterSelfClosingHTMLCodeElement(t *testing.T) {
	t.Parallel()

	source := `<code /> [visible](page.md) <pre/> <img src="image.png">`
	assert.Equal(t, []string{"page.md", "image.png"}, destinations(source))
}

func TestRangesIgnoresEmptyHTMLResourceAttributes(t *testing.T) {
	t.Parallel()

	source := `<a href="">empty</a><img src=''><a href="page.md">page</a>`
	assert.Equal(t, []string{"page.md"}, destinations(source))
}

func TestRewritePreservesSyntaxAroundDestinations(t *testing.T) {
	t.Parallel()

	source := `[page](old.md "title") and <img src="old.png" alt="old">`
	rewritten, err := Rewrite(source, func(destination string) (string, bool, error) {
		return "archive/" + destination, true, nil
	})
	require.NoError(t, err)
	assert.Equal(t, `[page](archive/old.md "title") and <img src="archive/old.png" alt="old">`, rewritten)
}

func destinations(source string) []string {
	ranges := Ranges(source)
	result := make([]string, len(ranges))
	for index, location := range ranges {
		result[index] = source[location.Start:location.End]
	}
	return result
}
