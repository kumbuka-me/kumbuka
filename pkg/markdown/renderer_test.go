package markdown

import (
	"context"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
	"github.com/kumbuka-me/sdk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWikiLinksAndCallouts(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "callouts")
	got, err := renderer.Render("See [[Postgres Restore|the runbook]].\n\n!!! warning\nDanger zone\n")

	require.NoError(t, err)
	assert.Contains(t, got, `href="/pages/postgres-restore"`)
	assert.Contains(t, got, `class="callout warning"`)
}

func TestWikiLinksSupportHeadingFragments(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t)
	got, err := renderer.Render("See [[operations/postgres#Restore from backup|restore procedure]].")

	require.NoError(t, err)
	assert.Contains(t, got, `href="/pages/operations/postgres#restore-from-backup"`)
	assert.Equal(t, []string{"operations/postgres"}, Links("[[operations/postgres#Restore from backup]]"))
}

func TestWikiLinkPrefix(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t)
	options := DefaultOptions()
	options.WikiLinkPrefix = "/docs/"
	got, err := renderer.RenderResolvedWithOptions("[[Hello World]]", func(target string) string {
		return Slug(target) + "/"
	}, options)

	require.NoError(t, err)
	assert.Contains(t, got, `href="/docs/hello-world/"`)
}

func TestLinksAreUnique(t *testing.T) {
	t.Parallel()

	got := Links("[[Hello World]] [[Hello World]] [[infra/DNS]]")

	assert.Equal(t, []string{"hello-world", "infra/dns"}, got)
}

func TestTabsRenderMarkdownPanels(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "tabs")
	source := `=== "Linux"

    **apt**

    ` + "```bash" + `
    apt install postgresql
    ` + "```" + `

=== "macOS"

    **brew** install postgresql
`

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `class="markdown-tabs"`)
	assert.Contains(t, got, `class="markdown-tab active"`)
	assert.Contains(t, got, `Linux`)
	assert.Contains(t, got, `macOS`)
	assert.Contains(t, got, `<strong>apt</strong>`)
	assert.Contains(t, got, `apt install postgresql`)
	assert.Contains(t, got, `<strong>brew</strong> install postgresql`)
}

func TestDetailsRenderMarkdownBody(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "details")
	source := `???+ "Why this works"

    This body contains **Markdown** and [[Another Page]].
`

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `<details class="markdown-details" open`)
	assert.Contains(t, got, `<summary>Why this works</summary>`)
	assert.Contains(t, got, `<strong>Markdown</strong>`)
	assert.Contains(t, got, `href="/pages/another-page"`)
}

func TestCustomBlocksAreIgnoredInsideFences(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "tabs", "details", "callouts")
	source := "```text\n=== \"Not a tab\"\n??? \"Not details\"\n!!! warning\n```\n"

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.NotContains(t, got, `class="markdown-tabs"`)
	assert.NotContains(t, got, `class="markdown-details"`)
	assert.NotContains(t, got, `class="callout`)
	assert.Contains(t, got, `===`)
	assert.Contains(t, got, `???`)
	assert.Contains(t, got, `!!! warning`)
}

func TestAdditionalCalloutKinds(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "callouts")
	got, err := renderer.Render("!!! info\nInformation\n\n!!! success\nWorked\n\n!!! danger\nStop\n")

	require.NoError(t, err)
	assert.Contains(t, got, `callout info`)
	assert.Contains(t, got, `callout success`)
	assert.Contains(t, got, `callout danger`)
}

func TestCalloutBodyRendersMarkdown(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "callouts")
	got, err := renderer.Render("!!! warning\n`$(VAR_NAME)` does not work with **envFrom**!\n")

	require.NoError(t, err)
	assert.Contains(t, got, `<code>$(VAR_NAME)</code>`)
	assert.Contains(t, got, `<strong>envFrom</strong>`)
	assert.NotContains(t, got, "`$(VAR_NAME)`")
}

func TestWikiLinksAreIgnoredInsideFencedCode(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t)
	source := "Before [[Real Page]]\n\n```text\n[[Literal Link]]\n```\n"

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `href="/pages/real-page"`)
	assert.NotContains(t, got, `href="/pages/literal-link"`)

	links := Links(source)

	assert.Equal(t, []string{"real-page"}, links)
}

func TestRenderPageExtractsHeadingTextWithoutHTMLMarkup(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t)
	rendered, err := renderer.RenderPageResolved("# Main *heading*\n\n## Child `code`\n", Slug)

	require.NoError(t, err)
	require.Len(t, rendered.Contents, 2)
	assert.Equal(t, 1, rendered.Contents[0].Level)
	assert.Equal(t, "Main heading", rendered.Contents[0].Title)
	assert.Equal(t, 2, rendered.Contents[1].Level)
	assert.Equal(t, "Child code", rendered.Contents[1].Title)
}

func TestSubpagesFunctionExpandsAtItsMarkdownPosition(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "subpages")
	rendered, err := renderer.RenderPageResolvedWithFunctions(
		"Before\n\n{{subpages}}\n\nAfter\n",
		Slug,
		DefaultOptions(),
		Functions{Capabilities: plugincap.Capabilities(nil, []sdk.NavigationNode{{Title: "Generated pages", URL: "/pages/child", Page: true}})},
	)

	require.NoError(t, err)
	assert.Contains(t, rendered.HTML, "Generated pages")
	assert.Less(t, strings.Index(rendered.HTML, "Before"), strings.Index(rendered.HTML, "Generated pages"))
	assert.Less(t, strings.Index(rendered.HTML, "Generated pages"), strings.Index(rendered.HTML, "After"))
	assert.NotContains(t, rendered.HTML, "{{subpages}}")
}

func TestSubpagesFunctionUsesCustomTitle(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "subpages")
	rendered, err := renderer.RenderPageResolvedWithFunctions(
		`{{subpages title="Related pages"}}`,
		Slug,
		DefaultOptions(),
		Functions{Capabilities: plugincap.Capabilities(nil, []sdk.NavigationNode{{Title: "Generated pages", URL: "/pages/child", Page: true}})},
	)

	require.NoError(t, err)
	assert.Contains(t, rendered.HTML, "<h2>Related pages</h2>")
}

func TestSubpagesFunctionAllowsHiddenTitle(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "subpages")
	rendered, err := renderer.RenderPageResolvedWithFunctions(
		`{{subpages title=""}}`,
		Slug,
		DefaultOptions(),
		Functions{Capabilities: plugincap.Capabilities(nil, []sdk.NavigationNode{{Title: "Generated pages", URL: "/pages/child", Page: true}})},
	)

	require.NoError(t, err)
	assert.Contains(t, rendered.HTML, "Generated pages")
	assert.NotContains(t, rendered.HTML, "<h2>")
}

func TestSubpagesFunctionLeavesUnsupportedOptionsLiteral(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "subpages")
	rendered, err := renderer.RenderPageResolvedWithFunctions(
		`{{subpages depth=2}}`,
		Slug,
		DefaultOptions(),
		Functions{Capabilities: plugincap.Capabilities(nil, []sdk.NavigationNode{{Title: "Generated pages", URL: "/pages/child", Page: true}})},
	)

	require.NoError(t, err)
	assert.Contains(t, rendered.HTML, "{{subpages depth=2}}")
	assert.NotContains(t, rendered.HTML, "Generated pages")
}

func TestSubpagesFunctionRemainsLiteralInsideFencedCode(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "subpages")
	rendered, err := renderer.RenderPageResolvedWithFunctions(
		"```markdown\n{{subpages}}\n```\n",
		Slug,
		DefaultOptions(),
		Functions{Capabilities: plugincap.Capabilities(nil, []sdk.NavigationNode{{Title: "Generated pages", URL: "/pages/child", Page: true}})},
	)

	require.NoError(t, err)
	assert.Contains(t, rendered.HTML, "{{subpages}}")
	assert.NotContains(t, rendered.HTML, "Generated pages")
}

func TestTableStyleDirective(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "tables")
	source := `| Service | Status | Owner |
| --- | --- | --- |
| API | Healthy | Platform |
| DB | Warning | Data |

{table header=accent col:2=info row:2=warning cell:2,2=danger}
`

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `kumbuka-table-styled`)
	assert.Contains(t, got, `table-tone-accent`)
	assert.Contains(t, got, `table-tone-info`)
	assert.Contains(t, got, `table-tone-warning`)
	assert.Contains(t, got, `table-tone-danger`)
	assert.NotContains(t, got, `{table`)
}

func TestConfluenceTablePalette(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "tables")
	source := `| Service | Status | Owner |
| --- | --- | --- |
| API | Healthy | Platform |
| DB | Warning | Data |

{table header=blue col:1=gray row:1=green cell:2,2=red}
`

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `table-tone-blue`)
	assert.Contains(t, got, `table-tone-gray`)
	assert.Contains(t, got, `table-tone-green`)
	assert.Contains(t, got, `table-tone-red`)
	assert.NotContains(t, got, `{table`)
}

func TestInteractiveTableDirective(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "tables")
	source := `| Service | Replicas |
| --- | ---: |
| API | 3 |
| DB | 1 |

{table sortable filterable}
`

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `kumbuka-table-sortable`)
	assert.Contains(t, got, `kumbuka-table-filterable`)
	assert.NotContains(t, got, `{table`)
}

func TestConfluenceInteractiveTableDirective(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "tables")
	source := `| Column 1 | Column 2 | Column 3 |
| --- | --- | --- |
| | | |
| | | |
| | | |

{table header=gray sortable filterable}
`

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `class="kumbuka-table-sortable kumbuka-table-filterable kumbuka-table-styled"`)
	assert.Contains(t, got, `class="table-tone-gray"`)
	assert.NotContains(t, got, `{table`)
}

func TestDisabledInteractiveTableDirectiveRemainsMarkdown(t *testing.T) {
	t.Parallel()

	renderer := isolatedTestRenderer(t, "tables")
	manager := renderer.PluginManager()
	require.NotNil(t, manager)
	require.NoError(t, manager.UpdateSettings(context.Background(), "me.kumbuka.tables", map[string]bool{
		"styles":    true,
		"sorting":   false,
		"filtering": false,
	}))

	source := `| Service | Replicas |
| --- | ---: |
| API | 3 |

{table sortable filterable}
`

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `{table sortable filterable}`)
	assert.NotContains(t, got, `kumbuka-table-sortable`)
	assert.NotContains(t, got, `kumbuka-table-filterable`)
}

func TestTableStyleDirectiveInsideTab(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "tabs", "tables")
	source := `=== "Status"

    | Service | Status |
    | --- | --- |
    | API | Healthy |

    {table header=accent cell:1,2=success}
`

	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `markdown-tabs`)
	assert.Contains(t, got, `table-tone-accent`)
	assert.Contains(t, got, `table-tone-success`)
	assert.NotContains(t, got, `{table`)
}

func TestRenderingOptionsControlCoreFeaturesOnly(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "callouts")
	options := DefaultOptions()
	options.WikiLinks = false

	source := `[[Runbook]]

!!! warning
Do not restart.
`
	got, err := renderer.RenderResolvedWithOptions(source, Slug, options)

	require.NoError(t, err)
	assert.NotContains(t, got, `href="/pages/runbook"`)
	assert.Contains(t, got, `class="callout warning"`)
}

func TestPluginSettingsDisableTableStyles(t *testing.T) {
	t.Parallel()

	renderer := isolatedTestRenderer(t, "tables")
	manager := renderer.PluginManager()
	require.NotNil(t, manager)
	require.NoError(t, manager.UpdateSettings(context.Background(), "me.kumbuka.tables", map[string]bool{
		"styles":    false,
		"sorting":   true,
		"filtering": true,
	}))

	source := `| A | B |
| --- | --- |
| one | two |

{table header=accent}
`
	got, err := renderer.Render(source)

	require.NoError(t, err)
	assert.Contains(t, got, `{table header=accent}`)
	assert.NotContains(t, got, `table-tone-accent`)
}

func TestFootnotesCanBeEnabled(t *testing.T) {
	t.Parallel()

	renderer := isolatedTestRenderer(t, "footnotes")
	manager := renderer.PluginManager()
	require.NotNil(t, manager)
	require.NoError(t, manager.Enable(context.Background(), "me.kumbuka.footnotes"))

	got, err := renderer.Render("Kumbuka has a note.[^1]\n\n[^1]: Stored with the page.\n")

	require.NoError(t, err)
	assert.Contains(t, got, `footnote`)
}

func TestDefinitionListsCanBeEnabled(t *testing.T) {
	t.Parallel()

	renderer := isolatedTestRenderer(t, "definition-lists")
	manager := renderer.PluginManager()
	require.NotNil(t, manager)
	require.NoError(t, manager.Enable(context.Background(), "me.kumbuka.definition-lists"))
	got, err := renderer.Render("Term\n: Definition\n")

	require.NoError(t, err)
	assert.Contains(t, got, `<dl>`)
}

func TestTaskListsOwnPresentationInPlugin(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "task-lists")
	got, err := renderer.Render("- [ ] pending\n- [x] complete\n")

	require.NoError(t, err)
	assert.Contains(t, got, `class="task-list-item"`)
	assert.Contains(t, got, `class="task-list-checkbox"`)
	assert.Contains(t, got, `aria-checked="false"`)
	assert.Contains(t, got, `>☐</span>`)
	assert.Contains(t, got, `class="task-list-checkbox checked"`)
	assert.Contains(t, got, `aria-checked="true"`)
	assert.Contains(t, got, `>☑</span>`)
	assert.NotContains(t, got, "<input")
}

func TestCodingLigaturesPreserveTypographerOperatorSequences(t *testing.T) {
	t.Parallel()

	renderer := isolatedTestRenderer(t, "typographer", "coding-ligatures")
	manager := renderer.PluginManager()
	require.NotNil(t, manager)
	require.NoError(t, manager.Enable(context.Background(), "me.kumbuka.typographer"))
	require.NoError(t, manager.Enable(context.Background(), "me.kumbuka.coding-ligatures"))

	got, err := renderer.Render(`"quoted" --> -> << >> ...`)

	require.NoError(t, err)
	assert.Contains(t, got, `“quoted”`)
	assert.Contains(t, got, `--&gt; -&gt; &lt;&lt; &gt;&gt;`)
	assert.Contains(t, got, `…`)
}

// TestSyntaxHighlightingEmitsChromaClasses verifies highlighted code exposes stable token classes for theme-aware CSS.
func TestSyntaxHighlightingEmitsChromaClasses(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "syntax-highlighting")
	got, err := renderer.Render("```go\nfunc main() { println(\"Kumbuka\") }\n```\n")

	require.NoError(t, err)
	assert.Contains(t, got, `data-kumbuka-code-block`)
	assert.Contains(t, got, `class="chroma"`)
	assert.Contains(t, got, `class="kd"`)
}

// TestSyntaxHighlightingCanBeDisabled verifies fenced code falls back to normal Markdown rendering.
func TestSyntaxHighlightingCanBeDisabled(t *testing.T) {
	t.Parallel()

	renderer := isolatedTestRenderer(t, "syntax-highlighting")
	require.NoError(t, renderer.PluginManager().Disable(context.Background(), "me.kumbuka.syntax-highlighting"))

	got, err := renderer.Render("```go\npackage main\n```\n")

	require.NoError(t, err)
	assert.Contains(t, got, `<pre><code class="language-go">`)
	assert.NotContains(t, got, `class="chroma"`)
}

// TestSyntaxHighlightingFallsBackForUnknownLanguage verifies providers can decline a fenced block.
func TestSyntaxHighlightingFallsBackForUnknownLanguage(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t, "syntax-highlighting")
	got, err := renderer.Render("```not-a-real-language\nplain <text>\n```\n")

	require.NoError(t, err)
	assert.Contains(t, got, `<pre><code class="language-not-a-real-language">plain &lt;text&gt;`)
	assert.NotContains(t, got, `class="chroma"`)
}

func TestUserMentionsRenderAsChips(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t)
	got, err := renderer.Render("@admin: please review with @Alice.\n")

	require.NoError(t, err)
	assert.Contains(t, got, `<span class="user-mention" data-kumbuka-mention>@admin</span>:`)
	assert.Contains(t, got, `<span class="user-mention" data-kumbuka-mention>@Alice.</span>`)
}

func TestUserMentionsStayLiteralInCodeAndEmailAddresses(t *testing.T) {
	t.Parallel()

	renderer := testRenderer(t)
	got, err := renderer.Render("Contact admin@example.com or run `echo @admin`.\n")

	require.NoError(t, err)
	assert.NotContains(t, got, `class="user-mention"`)
	assert.NotContains(t, got, `data-kumbuka-mention`)
	assert.Contains(t, got, `admin@example.com`)
	assert.Contains(t, got, `<code>echo @admin</code>`)
}
