package markdown

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/stretchr/testify/require"
)

var largePageWordCounts = []int{
	10,
	100,
	1_000,
	10_000,
	25_000,
	50_000,
	75_000,
	100_000,
	250_000,
	500_000,
	750_000,
	1_000_000,
}

var largePagePlugins = []string{
	"callouts",
	"details",
	"strikethrough",
	"syntax-highlighting",
	"tables",
	"task-lists",
}

var largePagePluginComparisonWordCounts = []int{
	10_000,
	100_000,
	250_000,
	500_000,
}

var largePagePluginComparisons = []struct {
	name    string
	plugins []string
}{
	{name: "core"},
	{name: "callouts", plugins: []string{"callouts"}},
	{name: "details", plugins: []string{"details"}},
	{name: "strikethrough", plugins: []string{"strikethrough"}},
	{name: "syntax-highlighting", plugins: []string{"syntax-highlighting"}},
	{name: "tables", plugins: []string{"tables"}},
	{name: "task-lists", plugins: []string{"task-lists"}},
	{name: "all", plugins: largePagePlugins},
}

const (
	// largePagePluginWireBytes mirrors the production WASM runtime's default
	// request/response wire limit. Very large plugin-enabled pages are skipped
	// by the render benchmark once their source approaches this boundary;
	// usage analysis and core rendering still cover the complete size matrix.
	largePagePluginWireBytes = 4 << 20

	// Keep some room for the JSON request envelope and plugin output expansion.
	largePagePluginWireReserve = 512 << 10
)

const largePagePluginSection = `## Deployment section %d

Lorem ipsum dolor sit amet, **consectetur adipiscing elit**. See [[platform/database-%d]] and [operations guide](https://example.com/operations).

!!! warning
Back up the database before deployment.

- [x] Review the plan
- [ ] Run the upgrade
- Keep ~~Friday~~ Monday as the preferred window

| Service | Status |
| --- | --- |
| API | Healthy |
| Database | Healthy |

{table header=accent col:2=info}

???+ "Deployment details"

    Validate monitoring, backups, alerts, and rollback instructions.

` + "```go" + `
func healthcheck() error {
	return nil
}
` + "```" + `

> Keep operational context near the documentation.
`

const largePageCoreSection = `## Documentation section %d

Lorem ipsum dolor sit amet, **consectetur adipiscing elit**, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.

See [[platform/database-%d]] for internal documentation and [the operations guide](https://example.com/operations) for external context.

- Review the deployment plan
- Confirm database backups
- Validate monitoring and alerts
- Prepare the rollback procedure

1. Open the change window
2. Deploy the application
3. Verify the service

` + "```go" + `
func healthcheck() error {
	return nil
}
` + "```" + `

> Keep operational context near the documentation so readers can act without searching elsewhere.
`

const largePageProseParagraph = `Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.`

var largePageFiller = []string{
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	"sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore", "et", "dolore",
	"magna", "aliqua", "enim", "ad", "minim", "veniam", "quis", "nostrud", "exercitation",
}

// BenchmarkLargePagePluginRender measures realistic Markdown rendering with
// common default-enabled first-party plugins. Each size gets a fresh plugin
// runtime so retained guest memory from a smaller case cannot affect the next
// size. Runtime resource boundaries are reported as skipped benchmark cases.
func BenchmarkLargePagePluginRender(b *testing.B) {
	for _, targetWords := range largePageWordCounts {
		b.Run(fmt.Sprintf("%d_words", targetWords), func(b *testing.B) {
			source := largePageSource(targetWords, true)
			require.Equal(b, targetWords, largePageWordCount(source))

			if largePagePluginSourceExceedsWireBudget(source) {
				b.Skipf(
					"source is %d bytes; production plugin wire limit is %d bytes",
					len(source),
					largePagePluginWireBytes,
				)
			}

			renderer := largePageBenchmarkRenderer(b, largePagePlugins...)
			benchmarkLargePagePluginRender(b, renderer, source, targetWords)
		})
	}
}

// BenchmarkLargePagePluginRenderByPlugin compares the same realistic large page
// against core rendering, each common first-party plugin in isolation, and the
// full plugin set. The selected sizes are large enough to expose nonlinear
// plugin costs without duplicating the complete size matrix for every plugin.
func BenchmarkLargePagePluginRenderByPlugin(b *testing.B) {
	for _, comparison := range largePagePluginComparisons {
		b.Run(comparison.name, func(b *testing.B) {
			for _, targetWords := range largePagePluginComparisonWordCounts {
				b.Run(fmt.Sprintf("%d_words", targetWords), func(b *testing.B) {
					source := largePageSource(targetWords, true)
					require.Equal(b, targetWords, largePageWordCount(source))

					if len(comparison.plugins) != 0 && largePagePluginSourceExceedsWireBudget(source) {
						b.Skipf(
							"source is %d bytes; production plugin wire limit is %d bytes",
							len(source),
							largePagePluginWireBytes,
						)
					}

					renderer := largePageBenchmarkRendererForPlugins(b, comparison.plugins...)
					benchmarkLargePagePluginRender(b, renderer, source, targetWords)
				})
			}
		})
	}
}

// BenchmarkLargePageCoreRender measures the core Markdown renderer over the
// complete size matrix, including pages too large for the current plugin WASM
// wire boundary.
func BenchmarkLargePageCoreRender(b *testing.B) {
	renderer := testRenderer(b)

	for _, targetWords := range largePageWordCounts {
		b.Run(fmt.Sprintf("%d_words", targetWords), func(b *testing.B) {
			source := largePageSource(targetWords, false)
			require.Equal(b, targetWords, largePageWordCount(source))

			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ReportMetric(float64(targetWords), "words")
			b.ReportMetric(float64(len(source)), "source_bytes")
			b.ResetTimer()

			for b.Loop() {
				rendered, err := renderer.RenderPageResolvedWithFunctions(
					source,
					Slug,
					DefaultOptions(),
					Functions{Context: context.Background(), Locale: "en"},
				)
				if err != nil {
					b.Fatal(err)
				}
				if len(rendered.HTML) == 0 {
					b.Fatal("large page rendered empty HTML")
				}
			}
		})
	}
}

// BenchmarkLargePageAnalyzeUsage isolates the cost of scanning realistic
// plugin-rich pages for active module usage over the complete size matrix.
func BenchmarkLargePageAnalyzeUsage(b *testing.B) {
	renderer := largePageBenchmarkRenderer(b, largePagePlugins...)

	for _, targetWords := range largePageWordCounts {
		b.Run(fmt.Sprintf("%d_words", targetWords), func(b *testing.B) {
			source := largePageSource(targetWords, true)
			require.Equal(b, targetWords, largePageWordCount(source))

			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ReportMetric(float64(targetWords), "words")
			b.ReportMetric(float64(len(source)), "source_bytes")
			b.ResetTimer()

			for b.Loop() {
				usage := renderer.AnalyzeUsage(source)
				if usage.Fingerprint == "" {
					b.Fatal("large page usage analysis returned an empty fingerprint")
				}
			}
		})
	}
}

// benchmarkLargePagePluginRender runs one prepared large-page render benchmark.
func benchmarkLargePagePluginRender(b *testing.B, renderer *Renderer, source string, targetWords int) {
	b.Helper()

	usage := renderer.AnalyzeUsage(source)

	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ReportMetric(float64(targetWords), "words")
	b.ReportMetric(float64(len(source)), "source_bytes")
	b.ResetTimer()

	for b.Loop() {
		rendered, err := renderer.RenderPageResolvedWithFunctions(source, Slug, DefaultOptions(), Functions{
			Context:     context.Background(),
			Locale:      "en",
			PluginUsage: &usage,
		})
		if err != nil {
			if largePagePluginResourceLimitError(err) {
				b.Skipf("plugin runtime boundary reached: %v", err)
			}
			b.Fatal(err)
		}
		if len(rendered.HTML) == 0 {
			b.Fatal("large page rendered empty HTML")
		}
	}
}

// largePagePluginSourceExceedsWireBudget reports whether a source is too close
// to the production WASM request/response limit for a useful plugin benchmark.
func largePagePluginSourceExceedsWireBudget(source string) bool {
	return len(source) >= largePagePluginWireBytes-largePagePluginWireReserve
}

// largePagePluginResourceLimitError reports expected sandbox resource
// boundaries separately from functional rendering failures.
func largePagePluginResourceLimitError(err error) bool {
	message := err.Error()
	for _, fragment := range []string{
		"out of memory",
		"plugin request exceeds size limit",
		"plugin response exceeds size limit",
		"response exceeds size limit",
		"too many fragments",
		"deadline exceeded",
	} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

// largePageBenchmarkRendererForPlugins returns core rendering when names is
// empty and a production-style plugin renderer otherwise.
func largePageBenchmarkRendererForPlugins(tb testing.TB, names ...string) *Renderer {
	tb.Helper()
	if len(names) == 0 {
		return NewWithRegistry(&plugin.Registry{})
	}
	return largePageBenchmarkRenderer(tb, names...)
}

// largePageBenchmarkRenderer returns a production-style AOT renderer with the
// requested bundled plugins. Unlike the normal plugin test helper, it does not
// force wazero's interpreter because these benchmarks are intended to measure
// realistic rendering cost.
func largePageBenchmarkRenderer(tb testing.TB, names ...string) *Renderer {
	tb.Helper()

	ctx := context.Background()
	runtime, err := wasm.New(
		ctx,
		wasm.Limits{InitTimeout: 30 * time.Second},
		wasm.WithPermissions("pages:read", "pages:content", "pages:write", "browser:render"),
	)
	require.NoError(tb, err)

	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime)
	tb.Cleanup(func() {
		require.NoError(tb, manager.Close(context.Background()))
	})

	archives := make([][]byte, 0, len(names))
	for _, name := range names {
		archive, readErr := plugins.Packages.ReadFile(name + ".kumbukaplugin")
		require.NoError(tb, readErr)
		archives = append(archives, archive)
	}

	require.NoError(tb, manager.Bootstrap(ctx, testDistribution(tb, archives)))
	return NewWithManager(registry, manager)
}

// largePageSource builds valid Markdown with realistic formatted sections and
// prose. Plugin-rich constructs are intentionally sparse: repeating a callout,
// table, details block, and task list every few dozen words is not realistic
// and would measure the plugin fragment safety guard rather than page size.
func largePageSource(targetWords int, pluginsEnabled bool) string {
	if targetWords <= 0 {
		return ""
	}

	var builder strings.Builder
	builder.Grow(targetWords * 7)

	remaining := targetWords
	sectionCount := largePageSectionCount(targetWords)

	for section := 1; section <= sectionCount; section++ {
		var block string
		if pluginsEnabled {
			block = fmt.Sprintf(largePagePluginSection, section, section)
		} else {
			block = fmt.Sprintf(largePageCoreSection, section, section)
		}

		words := largePageWordCount(block)
		if words > remaining {
			break
		}

		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(block)
		remaining -= words
	}

	paragraphWords := largePageWordCount(largePageProseParagraph)
	for remaining >= paragraphWords {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(largePageProseParagraph)
		remaining -= paragraphWords
	}

	if remaining > 0 {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		for index := 0; index < remaining; index++ {
			if index > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteString(largePageFiller[index%len(largePageFiller)])
		}
		builder.WriteByte('\n')
	}

	return builder.String()
}

// largePageSectionCount increases formatting variety with page size while
// keeping executable plugin fragments comfortably below the production limit.
func largePageSectionCount(targetWords int) int {
	if targetWords < 1_000 {
		return 0
	}

	sections := 1 + targetWords/50_000
	if sections > 24 {
		sections = 24
	}
	return sections
}

// largePageWordCount returns the whitespace-delimited word count used by the
// large-page benchmark scenarios.
func largePageWordCount(source string) int {
	return len(strings.Fields(source))
}
