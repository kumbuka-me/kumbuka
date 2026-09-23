package markdown

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
)

func TestNewPageRenderPlanSelectsSourceAwareBindings(t *testing.T) {
	t.Parallel()

	plan := &plugin.RenderPlan{
		Preprocessors: []plugin.PreprocessorBinding{
			{Selector: plugin.RenderSelector{ModuleID: "always"}},
			{Selector: plugin.RenderSelector{ModuleID: "used", UsageKey: "plugin:used", SourceAware: true}},
			{Selector: plugin.RenderSelector{ModuleID: "unused", UsageKey: "plugin:unused", SourceAware: true}},
		},
		Macros: map[string]plugin.MacroBinding{
			"used":   {Selector: plugin.RenderSelector{UsageKey: "macro:used", SourceAware: true}},
			"unused": {Selector: plugin.RenderSelector{UsageKey: "macro:unused", SourceAware: true}},
		},
	}

	result := newPageRenderPlan(plan, usageSet{"plugin:used": true, "macro:used": true}, nil)

	assert.Len(t, result.preprocessors, 2)
	assert.Equal(t, "always", result.preprocessors[0].Selector.ModuleID)
	assert.Equal(t, "used", result.preprocessors[1].Selector.ModuleID)
	assert.Contains(t, result.macros, "used")
	assert.NotContains(t, result.macros, "unused")
}

func TestNewPageRenderPlanSelectsContentPreprocessorForExportParameters(t *testing.T) {
	t.Parallel()

	selector := plugin.RenderSelector{PluginID: "example", ModuleID: "values", UsageKey: "values", SourceAware: true}
	plan := &plugin.RenderPlan{ContentPreprocessors: []plugin.ContentPreprocessorBinding{{Selector: selector}}}
	parameters := map[string]map[string]map[string]string{
		"example": {"values": {"region": "eu"}},
	}

	result := newPageRenderPlan(plan, nil, parameters)

	assert.Len(t, result.contentPreprocessors, 1)
	assert.Equal(t, selector, result.contentPreprocessors[0].Selector)
}

func TestNewPageRenderPlanSkipsUnusedCodeHighlighter(t *testing.T) {
	t.Parallel()

	plan := &plugin.RenderPlan{CodeHighlighter: &plugin.CodeHighlighterBinding{Selector: plugin.RenderSelector{UsageKey: "code", SourceAware: true}}}

	result := newPageRenderPlan(plan, nil, nil)

	assert.Nil(t, result.codeHighlighter)
}
