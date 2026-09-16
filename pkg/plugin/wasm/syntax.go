package wasm

import (
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// syntaxModule selects a public standard grammar through the same manifest for
// every distribution source. Feature flags control each fresh parser instance.
type syntaxModule struct {
	// owner, id, and syntax store the corresponding values for syntax module.
	owner, id, syntax string
	// usage contains the usage associated with syntax module.
	usage []plugin.SourceUsageRule
}

// SourceUsage exposes the manifest selectors for this host grammar.
func (m syntaxModule) SourceUsage() plugin.SourceUsage {
	return plugin.SourceUsage{ModuleID: m.id, Rules: m.usage}
}

// Extension returns the Markdown extension that delegates code highlighting to a plugin.
func (m syntaxModule) Extension(ctx plugin.Context) goldmark.Extender {
	if enabled, ok := ctx.Features[m.owner]; ok && !enabled {
		return noSyntax{}
	}
	if enabled, ok := ctx.Features[m.owner+"."+m.id]; ok && !enabled {
		return noSyntax{}
	}
	return map[string]goldmark.Extender{
		"tables": extension.Table, "strikethrough": extension.Strikethrough,
		"task-list": extension.TaskList, "definition-list": extension.DefinitionList,
		"footnote": extension.Footnote, "linkify": extension.Linkify,
	}[m.syntax]
}

// noSyntax disables built-in syntax parsing so plugin highlighting can own code blocks.
type noSyntax struct{}

// Extend registers the no-syntax parser behavior with a Goldmark instance.
func (noSyntax) Extend(goldmark.Markdown) {}
