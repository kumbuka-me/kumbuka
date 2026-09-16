package markdown

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xhtml "golang.org/x/net/html"
)

func TestImageWidthsRenderInMarkdown(t *testing.T) {
	t.Parallel()

	t.Run("bare pixels", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render(`![Diagram](diagram.png){width=640}`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:640px", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("explicit pixels", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render(`![Diagram](diagram.png){width=640px}`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:640px", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("percentage", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render(`![Diagram](diagram.png){width=50%}`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("linked image", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render(`[![Diagram](diagram.png){width=50%}](full.png)`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("reference image", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render("![Diagram][image]{width=50%}\n\n[image]: diagram.png")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("collapsed reference", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render("![Diagram][]{width=640}\n\n[Diagram]: diagram.png")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:640px", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("shortcut reference", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render("![Diagram]{width=640}\n\n[Diagram]: diagram.png")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:640px", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("emphasis", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render(`**![Diagram](diagram.png){width=50%}**`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("blockquote", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render(`> ![Diagram](diagram.png){width=50%}`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("list", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t).Render(`- ![Diagram](diagram.png){width=50%}`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("table", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t, "tables").Render("| Diagram |\n| --- |\n| ![Diagram](diagram.png){width=50%} |")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("callout", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t, "callouts").Render("!!! info\n![Diagram](diagram.png){width=50%}\n")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("tab", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t, "tabs").Render("=== \"Diagram\"\n\n    ![Diagram](diagram.png){width=50%}\n")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})

	t.Run("details", func(t *testing.T) {
		t.Parallel()

		got, err := testRenderer(t, "details").Render("??? \"Diagram\"\n\n    ![Diagram](diagram.png){width=50%}\n")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
		assert.Equal(t, "diagram.png", images[0]["src"])
		assert.Equal(t, "Diagram", images[0]["alt"])
		assert.NotContains(t, got, "{width=")
	})
}

func TestImageWidthsKeepTitlesURLsAndFollowingText(t *testing.T) {
	t.Parallel()

	got, err := testRenderer(t).Render(`Before ![A & B](images/diagram(v2).png "Overview & details"){width=640} between ![Other](other.png){width=25%} after.`)
	require.NoError(t, err)
	images := renderedImageAttributes(t, got)
	require.Len(t, images, 2)
	assert.Equal(t, "images/diagram(v2).png", images[0]["src"])
	assert.Equal(t, "A & B", images[0]["alt"])
	assert.Equal(t, "Overview & details", images[0]["title"])
	assert.Equal(t, "width:640px", normalizedImageStyle(images[0]["style"]))
	assert.Equal(t, "width:25%", normalizedImageStyle(images[1]["style"]))
	assert.Contains(t, got, "Before ")
	assert.Contains(t, got, " between ")
	assert.Contains(t, got, " after.")
}

func TestAdjacentImageWidths(t *testing.T) {
	t.Parallel()

	got, err := testRenderer(t).Render(`![A](a.png){width=30%}![B](b.png){width=40%}`)
	require.NoError(t, err)
	images := renderedImageAttributes(t, got)
	require.Len(t, images, 2)
	assert.Equal(t, "width:30%", normalizedImageStyle(images[0]["style"]))
	assert.Equal(t, "width:40%", normalizedImageStyle(images[1]["style"]))
}

func TestImageWidthPreservesLineBreaks(t *testing.T) {
	t.Parallel()

	t.Run("soft break", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=50%}\nFollowing")
		require.NoError(t, err)
		assert.Contains(t, strings.ReplaceAll(got, "/>", ">"), ">\nFollowing")
		assert.NotContains(t, got, "{width=")
	})

	t.Run("two spaces", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=50%}  \nFollowing")
		require.NoError(t, err)
		assert.Contains(t, strings.ReplaceAll(got, "/>", ">"), "<br>\nFollowing")
		assert.NotContains(t, got, "{width=")
	})

	t.Run("backslash", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=50%}\\\nFollowing")
		require.NoError(t, err)
		assert.Contains(t, strings.ReplaceAll(got, "/>", ">"), "<br>\nFollowing")
		assert.NotContains(t, got, "{width=")
	})
}

func TestInvalidImageWidthsStayVisible(t *testing.T) {
	t.Parallel()

	t.Run("zero width", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=0}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=0}")
	})

	t.Run("negative width", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=-1}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=-1}")
	})

	t.Run("percentage above maximum", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=101%}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=101%}")
	})

	t.Run("pixels above maximum", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=10001}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=10001}")
	})

	t.Run("fractional percentage", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=50.5%}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50.5%}")
	})

	t.Run("unsupported unit", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=10em}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=10em}")
	})

	t.Run("multiple dimensions", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=50% height=20}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50% height=20}")
	})

	t.Run("extra CSS declaration", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=50%;position:fixed}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%;position:fixed}")
	})

	t.Run("height instead of width", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){height=20}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{height=20}")
	})

	t.Run("unclosed directive", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png){width=50%")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%")
	})
}

func TestImageWidthOnlyConsumesAdjacentUnescapedDirectives(t *testing.T) {
	t.Parallel()

	t.Run("leading space", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png) {width=50%}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("leading newline", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png)\n{width=50%}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("separate paragraph", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png)\n\n{width=50%}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("escaped opening brace", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png)\\{width=50%}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("encoded opening brace", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("![Diagram](diagram.png)&#123;width=50%}")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%}")
	})
}

func TestImageWidthDoesNotInterpretCodeOrOrdinaryLinks(t *testing.T) {
	t.Parallel()

	t.Run("inline code", func(t *testing.T) {
		t.Parallel()
		renderer := testRenderer(t)
		got, err := renderer.Render("`![Diagram](diagram.png){width=50%}`")
		require.NoError(t, err)
		assert.Empty(t, renderedImageAttributes(t, got))
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("backtick code fence", func(t *testing.T) {
		t.Parallel()
		renderer := testRenderer(t)
		got, err := renderer.Render("```text\n![Diagram](diagram.png){width=50%}\n```")
		require.NoError(t, err)
		assert.Empty(t, renderedImageAttributes(t, got))
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("tilde code fence", func(t *testing.T) {
		t.Parallel()
		renderer := testRenderer(t)
		got, err := renderer.Render("~~~text\n![Diagram](diagram.png){width=50%}\n~~~")
		require.NoError(t, err)
		assert.Empty(t, renderedImageAttributes(t, got))
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("indented code", func(t *testing.T) {
		t.Parallel()
		renderer := testRenderer(t)
		got, err := renderer.Render("    ![Diagram](diagram.png){width=50%}\n")
		require.NoError(t, err)
		assert.Empty(t, renderedImageAttributes(t, got))
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("escaped image", func(t *testing.T) {
		t.Parallel()
		renderer := testRenderer(t)
		got, err := renderer.Render(`\![Diagram](diagram.png){width=50%}`)
		require.NoError(t, err)
		assert.Empty(t, renderedImageAttributes(t, got))
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("ordinary link", func(t *testing.T) {
		t.Parallel()
		renderer := testRenderer(t)
		got, err := renderer.Render(`[Diagram](diagram.png){width=50%}`)
		require.NoError(t, err)
		assert.Empty(t, renderedImageAttributes(t, got))
		assert.Contains(t, got, "{width=50%}")
	})
}

func TestImageWidthDoesNotInterpretRawHTMLOrNestedAltText(t *testing.T) {
	t.Parallel()

	t.Run("raw HTML image", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render(`<img src="diagram.png" alt="Diagram">{width=50%}`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("nested image alt text", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render(`![Outer ![Inner](inner.png){width=50%}](outer.png)`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%}")
	})

	t.Run("literal directive in alt text", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render(`![Diagram {width=50%}](diagram.png)`)
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
		assert.Contains(t, got, "{width=50%}")
	})
}

func TestImageWidthDoesNotRequireOptionalRenderingFeatures(t *testing.T) {
	t.Parallel()

	got, err := testRenderer(t).RenderResolvedWithOptions(`![Diagram](diagram.png){width=50%}`, Slug, Options{})
	require.NoError(t, err)
	images := renderedImageAttributes(t, got)
	require.Len(t, images, 1)
	assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
}

func TestUnsizedImageRenderingIsUnchanged(t *testing.T) {
	t.Parallel()

	got, err := testRenderer(t).Render(`![Diagram](diagram.png "Title")`)
	require.NoError(t, err)
	images := renderedImageAttributes(t, got)
	require.Len(t, images, 1)
	assert.Equal(t, map[string]string{
		"src": "diagram.png", "alt": "Diagram", "title": "Title",
	}, images[0])
}

func TestImageWidthSanitizerAllowsOnlyBoundedWidths(t *testing.T) {
	t.Parallel()

	got, err := testRenderer(t).Render(`<img src="diagram.png" style="width:50%;position:fixed;top:0;height:1px;background:url(https://example.test/track)" onerror="alert(1)"><span style="width:50%">Text</span>`)
	require.NoError(t, err)
	images := renderedImageAttributes(t, got)
	require.Len(t, images, 1)
	assert.Equal(t, "width:50%", normalizedImageStyle(images[0]["style"]))
	assert.NotContains(t, got, "onerror")
	assert.NotContains(t, got, "position")
	assert.NotContains(t, got, "background")
	assert.NotContains(t, got, "height:")
	assert.Equal(t, 1, strings.Count(got, `style="`), "width styles must not be allowed on spans")

	t.Run("zero pixels", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("<img src=\"diagram.png\" style=\"width:0px\">")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
	})

	t.Run("percentage above maximum", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("<img src=\"diagram.png\" style=\"width:101%\">")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
	})

	t.Run("pixels above maximum", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("<img src=\"diagram.png\" style=\"width:10001px\">")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
	})

	t.Run("CSS expression", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("<img src=\"diagram.png\" style=\"width:expression(alert(1))\">")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
	})

	t.Run("CSS calculation", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("<img src=\"diagram.png\" style=\"width:calc(50% + 1px)\">")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
	})

	t.Run("CSS variable", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("<img src=\"diagram.png\" style=\"width:var(--width)\">")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
	})

	t.Run("CSS URL", func(t *testing.T) {
		t.Parallel()
		got, err := testRenderer(t).Render("<img src=\"diagram.png\" style=\"width:url(https://example.test/track)\">")
		require.NoError(t, err)
		images := renderedImageAttributes(t, got)
		require.Len(t, images, 1)
		assert.Empty(t, images[0]["style"])
	})
}

func TestImageWidthKeepsURLAndAttributeSanitization(t *testing.T) {
	t.Parallel()

	got, err := testRenderer(t).Render(`![Diagram](javascript:alert%281%29){width=50%}`)
	require.NoError(t, err)
	assert.NotContains(t, got, "javascript:")

	got, err = testRenderer(t).Render(`![Diagram](diagram.png){width=50% onerror="alert(1)"}`)
	require.NoError(t, err)
	images := renderedImageAttributes(t, got)
	require.Len(t, images, 1)
	assert.Empty(t, images[0]["style"])
	assert.Empty(t, images[0]["onerror"])
	assert.Contains(t, got, "{width=")
}

// renderedImageAttributes reads actual image attributes rather than text or code examples.
func renderedImageAttributes(t *testing.T, rendered string) []map[string]string {
	t.Helper()
	var images []map[string]string
	tokens := xhtml.NewTokenizer(strings.NewReader(rendered))
	for {
		kind := tokens.Next()
		if kind == xhtml.ErrorToken {
			require.ErrorIs(t, tokens.Err(), io.EOF)
			return images
		}
		if kind != xhtml.StartTagToken && kind != xhtml.SelfClosingTagToken {
			continue
		}
		token := tokens.Token()
		if token.Data != "img" {
			continue
		}
		attributes := make(map[string]string, len(token.Attr))
		for _, attribute := range token.Attr {
			attributes[attribute.Key] = attribute.Val
		}
		images = append(images, attributes)
	}
}

func normalizedImageStyle(value string) string {
	return strings.TrimSuffix(strings.ReplaceAll(value, " ", ""), ";")
}
