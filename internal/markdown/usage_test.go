package markdown

import (
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type usagePreprocessor struct{ usage plugin.SourceUsage }

func (m usagePreprocessor) SourceUsage() plugin.SourceUsage { return m.usage }
func (usagePreprocessor) Preprocess(_ plugin.Context, source string) (string, error) {
	return source, nil
}

type usageMacro struct{ usage plugin.SourceUsage }

func (m usageMacro) SourceUsage() plugin.SourceUsage                        { return m.usage }
func (usageMacro) Name() string                                             { return "pages" }
func (usageMacro) Parse(string) (plugin.Invocation, bool)                   { return nil, false }
func (usageMacro) Render(plugin.Context, plugin.Invocation) (string, error) { return "", nil }

func TestAnalyzeUsageExtractsValuesAndIgnoresFencedMacros(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		Preprocessors: []plugin.Preprocessor{usagePreprocessor{usage: plugin.SourceUsage{ModuleID: "vars", Rules: []plugin.SourceUsageRule{{Substitution: "var"}}}}},
		Macros:        []plugin.Macro{usageMacro{usage: plugin.SourceUsage{ModuleID: "pages", Rules: []plugin.SourceUsageRule{{Macro: "pages"}}}}},
	}))
	plan, release := registry.AcquireRenderPlan()
	defer release()
	index := analyzeUsage("{{var:environment}}\n\n{{pages query=\"prod\"}}\n\n```\n{{pages query=\"ignored\"}}\n```\n", plan)
	require.Len(t, index.Modules, 2)
	assert.Equal(t, []string{"environment"}, index.Modules[0].Values)
	assert.Equal(t, []string{"query=\"prod\""}, index.Modules[1].Values)
}

func TestAnalyzeUsageDetectsFenceLanguage(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		Preprocessors: []plugin.Preprocessor{usagePreprocessor{usage: plugin.SourceUsage{ModuleID: "diagram", Rules: []plugin.SourceUsageRule{{Fence: "mermaid"}}}}},
	}))
	plan, release := registry.AcquireRenderPlan()
	defer release()
	index := analyzeUsage("````mermaid\ngraph TD\n````\n", plan)
	require.Len(t, index.Modules, 1)
	assert.Equal(t, []string{"mermaid"}, index.Modules[0].Values)
}

type countingUsagePreprocessor struct {
	usage     plugin.SourceUsage
	calls     *int
	transform func(string) string
}

func (m countingUsagePreprocessor) SourceUsage() plugin.SourceUsage { return m.usage }
func (m countingUsagePreprocessor) Preprocess(_ plugin.Context, source string) (string, error) {
	*m.calls = *m.calls + 1
	if m.transform != nil {
		source = m.transform(source)
	}
	return source, nil
}

type countingUsageMacro struct{ calls *int }

func (m countingUsageMacro) SourceUsage() plugin.SourceUsage {
	return plugin.SourceUsage{ModuleID: "pages", Rules: []plugin.SourceUsageRule{{Macro: "pages"}}}
}
func (countingUsageMacro) Name() string { return "pages" }
func (m countingUsageMacro) Parse(source string) (plugin.Invocation, bool) {
	*m.calls = *m.calls + 1
	if strings.TrimSpace(source) != "{{pages}}" {
		return nil, false
	}
	return plugin.Invocation(`{}`), true
}
func (countingUsageMacro) Render(plugin.Context, plugin.Invocation) (string, error) {
	return "<p>pages</p>", nil
}

type usageContentPreprocessor struct {
	usage     plugin.SourceUsage
	priority  int
	calls     *int
	transform func(string) string
}

func (m usageContentPreprocessor) SourceUsage() plugin.SourceUsage { return m.usage }
func (m usageContentPreprocessor) Priority() int                   { return m.priority }
func (m usageContentPreprocessor) PreprocessContent(_ plugin.Context, source string) (plugin.PreparedContent, error) {
	*m.calls = *m.calls + 1
	if m.transform != nil {
		source = m.transform(source)
	}
	return plugin.PreparedContent{Markdown: source}, nil
}

func TestRenderSkipsUnusedSourceAwarePreprocessor(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	calls := 0
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		Preprocessors: []plugin.Preprocessor{countingUsagePreprocessor{
			usage: plugin.SourceUsage{ModuleID: "callouts", Rules: []plugin.SourceUsageRule{{Contains: "!!! "}}},
			calls: &calls,
		}},
	}))
	renderer := NewWithRegistry(registry)

	_, err := renderer.Render("plain Markdown")
	require.NoError(t, err)
	assert.Zero(t, calls)

	_, err = renderer.Render("!!! warning\ncontent")
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestPreprocessorReanalyzesTransformedMarkdown(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	firstCalls := 0
	secondCalls := 0
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		Preprocessors: []plugin.Preprocessor{
			countingUsagePreprocessor{
				usage: plugin.SourceUsage{ModuleID: "first", Rules: []plugin.SourceUsageRule{{Contains: "alpha"}}},
				calls: &firstCalls,
				transform: func(string) string {
					return "beta"
				},
			},
			countingUsagePreprocessor{
				usage: plugin.SourceUsage{ModuleID: "second", Rules: []plugin.SourceUsageRule{{Contains: "beta"}}},
				calls: &secondCalls,
			},
		},
	}))
	renderer := NewWithRegistry(registry)

	_, err := renderer.Render("alpha")
	require.NoError(t, err)
	assert.Equal(t, 1, firstCalls)
	assert.Equal(t, 1, secondCalls)
}

type countingUsagePostprocessor struct {
	usage plugin.SourceUsage
	calls *int
}

func (m countingUsagePostprocessor) SourceUsage() plugin.SourceUsage { return m.usage }
func (m countingUsagePostprocessor) Postprocess(_ plugin.Context, source string) (string, error) {
	*m.calls = *m.calls + 1
	return source, nil
}

func TestPostprocessorUsesTransformedMarkdownUsage(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	preprocessorCalls := 0
	postprocessorCalls := 0
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		Preprocessors: []plugin.Preprocessor{countingUsagePreprocessor{
			usage:     plugin.SourceUsage{ModuleID: "producer", Rules: []plugin.SourceUsageRule{{Contains: "alpha"}}},
			calls:     &preprocessorCalls,
			transform: func(string) string { return "beta" },
		}},
		Postprocessors: []plugin.Postprocessor{countingUsagePostprocessor{
			usage: plugin.SourceUsage{ModuleID: "consumer", Rules: []plugin.SourceUsageRule{{Contains: "beta"}}},
			calls: &postprocessorCalls,
		}},
	}))
	renderer := NewWithRegistry(registry)

	_, err := renderer.Render("alpha")
	require.NoError(t, err)
	assert.Equal(t, 1, preprocessorCalls)
	assert.Equal(t, 1, postprocessorCalls)
}

func TestSourceAwareMacroOnlyParsesCandidateLines(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	calls := 0
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		Macros: []plugin.Macro{countingUsageMacro{calls: &calls}},
	}))
	renderer := NewWithRegistry(registry)
	source := strings.Repeat("ordinary line\n", 100) + "{{pages}}\n" + strings.Repeat("another line\n", 100)

	got, err := renderer.Render(source)
	require.NoError(t, err)
	assert.Contains(t, got, "<p>pages</p>")
	assert.Equal(t, 1, calls)
}

func TestContentPreprocessorReanalyzesTransformedMarkdown(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	includeCalls := 0
	variableCalls := 0
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		ContentPreprocessors: []plugin.ContentPreprocessor{
			usageContentPreprocessor{
				usage:    plugin.SourceUsage{ModuleID: "include", Rules: []plugin.SourceUsageRule{{Contains: "{{include:"}}},
				priority: 100,
				calls:    &includeCalls,
				transform: func(string) string {
					return "{{var:environment}}"
				},
			},
			usageContentPreprocessor{
				usage:    plugin.SourceUsage{ModuleID: "variables", Rules: []plugin.SourceUsageRule{{Substitution: "var"}}},
				priority: 200,
				calls:    &variableCalls,
			},
		},
	}))
	renderer := NewWithRegistry(registry)

	_, err := renderer.Render("{{include:shared/header}}")
	require.NoError(t, err)
	assert.Equal(t, 1, includeCalls)
	assert.Equal(t, 1, variableCalls)
}

func TestStalePluginUsageFallsBackToTransientAnalysis(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	calls := 0
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		Preprocessors: []plugin.Preprocessor{countingUsagePreprocessor{
			usage: plugin.SourceUsage{ModuleID: "callouts", Rules: []plugin.SourceUsageRule{{Contains: "!!! "}}},
			calls: &calls,
		}},
	}))
	renderer := NewWithRegistry(registry)
	source := "!!! warning\ncontent"
	stale := renderer.AnalyzeUsage(source)
	stale.Fingerprint = "stale"

	_, err := renderer.RenderPageResolvedWithFunctions(
		source,
		Slug,
		DefaultOptions(),
		Functions{PluginUsage: &stale},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestPluginUsageForDifferentSourceFallsBackToTransientAnalysis(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	calls := 0
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		Preprocessors: []plugin.Preprocessor{countingUsagePreprocessor{
			usage: plugin.SourceUsage{ModuleID: "callouts", Rules: []plugin.SourceUsageRule{{Contains: "!!! "}}},
			calls: &calls,
		}},
	}))
	renderer := NewWithRegistry(registry)
	index := renderer.AnalyzeUsage("plain Markdown")

	_, err := renderer.RenderPageResolvedWithFunctions(
		"!!! warning\ncontent",
		Slug,
		DefaultOptions(),
		Functions{PluginUsage: &index},
	)

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestContentPreprocessorWithExportParametersRunsWithoutSourceMatch(t *testing.T) {
	t.Parallel()
	registry := &plugin.Registry{}
	calls := 0
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example", Name: "Example"}, plugin.Contributions{
		ContentPreprocessors: []plugin.ContentPreprocessor{usageContentPreprocessor{
			usage:    plugin.SourceUsage{ModuleID: "variables", Rules: []plugin.SourceUsageRule{{Substitution: "var"}}},
			priority: 200,
			calls:    &calls,
		}},
	}))
	renderer := NewWithRegistry(registry)

	_, err := renderer.RenderPageResolvedWithFunctions(
		"plain Markdown",
		Slug,
		DefaultOptions(),
		Functions{ExportParameters: map[string]map[string]map[string]string{
			"io.example": {"variables": {"unused": "temporary"}},
		}},
	)

	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestRequiredPluginIDsUsesManifestUsageWithoutRuntime(t *testing.T) {
	t.Parallel()
	manifests := []pluginpackage.Manifest{
		{
			ID: "me.kumbuka.mermaid",
			Modules: []pluginpackage.Module{{
				Type:  "renderer-extension",
				ID:    "fences",
				Usage: []pluginpackage.UsageRule{{Fence: "mermaid"}},
			}},
		},
		{
			ID: "me.kumbuka.tables",
			Modules: []pluginpackage.Module{{
				Type:  "markdown-syntax",
				ID:    "tables",
				Usage: []pluginpackage.UsageRule{{Contains: "|"}},
			}},
		},
		{
			ID: "me.kumbuka.typographer",
			Modules: []pluginpackage.Module{{
				Type: "renderer-extension",
				ID:   "typographer",
			}},
		},
		{
			ID: "me.kumbuka.browser-only",
			Modules: []pluginpackage.Module{{
				Type: "browser-module",
				ID:   "browser",
			}},
		},
	}

	ids := RequiredPluginIDs([]string{"```mermaid\ngraph TD\n```"}, manifests)
	assert.Equal(t, []string{"me.kumbuka.mermaid", "me.kumbuka.typographer"}, ids)
}
