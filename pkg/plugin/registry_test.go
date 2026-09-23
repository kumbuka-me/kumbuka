package plugin

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegistryRegistrationIsAtomicAndReversible verifies registry registration is atomic and reversible behavior.
func TestRegistryRegistrationIsAtomicAndReversible(t *testing.T) {
	r := &Registry{}
	descriptor := Descriptor{ID: "base", Name: "Base"}
	macro := BoundMacro[string]{MacroName: "example", ParseOptions: func(s string) (string, bool) { return s, true }}
	modules := Contributions{Macros: []Macro{macro}, BrowserModules: []BrowserModule{{ID: "browser", JavaScript: "assets/plugin.js"}}}
	require.NoError(t, r.Register(descriptor, modules))
	modules.Macros[0] = nil
	modules.BrowserModules[0].JavaScript = "mutated"
	snapshot := r.Snapshot()
	require.Equal(t, "example", snapshot.Entries[0].Contributions.Macros[0].Name())
	require.Equal(t, "assets/plugin.js", snapshot.Entries[0].Contributions.BrowserModules[0].JavaScript)
	require.Error(t, r.Register(descriptor, Contributions{}))
	require.Error(t, r.Register(Descriptor{ID: "collision", Name: "Collision"}, Contributions{Macros: []Macro{macro}}))
	require.Error(t, r.Register(Descriptor{ID: "missing", Name: "Missing", Requires: []string{"absent"}}, Contributions{}))
	require.Len(t, r.Snapshot().Entries, 1)
	dependency := Descriptor{ID: "dependent", Name: "Dependent", Requires: []string{"base"}}
	require.NoError(t, r.Register(dependency, Contributions{}))
	dependency.Requires[0] = "mutated"
	require.Error(t, r.Unregister("base"))
	require.NoError(t, r.Unregister("dependent"))
	require.NoError(t, r.Unregister("base"))
	require.Empty(t, r.Snapshot().Entries)
	require.Len(t, snapshot.Entries, 1)
	require.NoError(t, r.Register(descriptor, Contributions{Macros: []Macro{macro}}))
	snapshot.Entries[0].Descriptor.Name = "mutated"
	require.Equal(t, "Base", r.Snapshot().Entries[0].Descriptor.Name)
}

// TestRegistryRejectsInvalidContributions verifies registry rejects invalid contributions behavior.
func TestRegistryRejectsInvalidContributions(t *testing.T) {
	for _, modules := range []Contributions{
		{Macros: []Macro{nil}},
		{Preprocessors: []Preprocessor{nil}},
		{MarkdownExtensions: []MarkdownExtension{nil}},
		{Postprocessors: []Postprocessor{nil}},
		{BrowserModules: []BrowserModule{{ID: "same"}, {ID: "same"}}},
		{EditorExtensions: []EditorExtension{{ID: "../bad"}}},
	} {
		r := &Registry{}
		require.Error(t, r.Register(Descriptor{ID: "test", Name: "Test"}, modules))
		require.Empty(t, r.Snapshot().Entries)
	}
}

// TestRegistryConcurrentSnapshotsAndRemoval verifies registry concurrent snapshots and removal behavior.
func TestRegistryConcurrentSnapshotsAndRemoval(t *testing.T) {
	r := &Registry{}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				_ = r.Snapshot()
			}
		})
	}
	for range 100 {
		require.NoError(t, r.Register(Descriptor{ID: "test", Name: "Test"}, Contributions{}))
		require.NoError(t, r.Unregister("test"))
	}
	wg.Wait()
}

// testCodeHighlighter provides test state for test code highlighter behavior.
type testCodeHighlighter struct{}

// Highlight implements CodeHighlighter for registry validation tests.
func (testCodeHighlighter) Highlight(Context, string, string) (CodeHighlightResult, error) {
	return CodeHighlightResult{Matched: true}, nil
}

// TestRegistryAllowsOnlyOneActiveCodeHighlighter verifies exclusive provider ownership.
func TestRegistryAllowsOnlyOneActiveCodeHighlighter(t *testing.T) {
	t.Parallel()

	r := &Registry{}
	first := Contributions{CodeHighlighters: []CodeHighlighterModule{{ID: "first", Highlighter: testCodeHighlighter{}}}}
	second := Contributions{CodeHighlighters: []CodeHighlighterModule{{ID: "second", Highlighter: testCodeHighlighter{}}}}

	require.NoError(t, r.Register(Descriptor{ID: "one", Name: "One"}, first))
	require.ErrorContains(t, r.Register(Descriptor{ID: "two", Name: "Two"}, second), "already provided")
	require.NoError(t, r.Unregister("one"))
	require.NoError(t, r.Register(Descriptor{ID: "two", Name: "Two"}, second))
}

// plannedContentPreprocessor provides test state for planned content preprocessor behavior.
type plannedContentPreprocessor struct {
	// priority configures or records the priority value used by the fixture.
	priority int
	// usage configures the usage used by the fixture.
	usage SourceUsage
}

func (m plannedContentPreprocessor) Priority() int            { return m.priority }
func (m plannedContentPreprocessor) SourceUsage() SourceUsage { return m.usage }
func (m plannedContentPreprocessor) PreprocessContent(_ Context, source string) (PreparedContent, error) {
	return PreparedContent{Markdown: source}, nil
}

// TestRegistryCachesImmutableRenderPlan verifies render-only flattening happens on
// lifecycle changes rather than once per page render.
func TestRegistryCachesImmutableRenderPlan(t *testing.T) {
	t.Parallel()

	r := &Registry{}
	macro := BoundMacro[string]{MacroName: "example", ParseOptions: func(s string) (string, bool) { return s, true }}
	require.NoError(t, r.Register(Descriptor{ID: "base", Name: "Base"}, Contributions{
		ContentPreprocessors: []ContentPreprocessor{
			plannedContentPreprocessor{priority: 200, usage: SourceUsage{ModuleID: "late", Rules: []SourceUsageRule{{Contains: "late"}}}},
			plannedContentPreprocessor{priority: 100, usage: SourceUsage{ModuleID: "early", Rules: []SourceUsageRule{{Contains: "early"}}}},
		},
		Macros: []Macro{macro},
	}))

	first, releaseFirst := r.AcquireRenderPlan()
	require.Len(t, first.ContentPreprocessors, 2)
	require.Equal(t, 100, first.ContentPreprocessors[0].Priority)
	require.Equal(t, 200, first.ContentPreprocessors[1].Priority)
	require.Contains(t, first.Macros, "example")
	require.Len(t, first.SourceUsage, 2)
	require.Equal(t, "late", first.SourceUsage[0].Usage.ModuleID)
	require.Equal(t, "early", first.SourceUsage[1].Usage.ModuleID)
	require.NotEmpty(t, first.UsageFingerprint)

	same, releaseSame := r.AcquireRenderPlan()
	require.Same(t, first, same)
	releaseSame()

	require.NoError(t, r.Register(Descriptor{ID: "other", Name: "Other"}, Contributions{}))
	second, releaseSecond := r.AcquireRenderPlan()
	require.NotSame(t, first, second)
	require.Greater(t, second.Generation, first.Generation)

	releaseSecond()
	releaseFirst()
}
