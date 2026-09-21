package wasm

import (
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/yuin/goldmark/v2/extension"
)

// syntaxModule selects a public standard grammar through the same manifest for every distribution source. Feature flags control each fresh parser instance.
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

// Components returns fresh Goldmark v2 parser and HTML renderer components for this syntax.
func (m syntaxModule) Components(ctx plugin.Context) plugin.MarkdownComponents {
	if enabled, ok := ctx.Features[m.owner]; ok && !enabled {
		return plugin.MarkdownComponents{}
	}
	if enabled, ok := ctx.Features[m.owner+"."+m.id]; ok && !enabled {
		return plugin.MarkdownComponents{}
	}

	switch m.syntax {
	case "tables":
		return plugin.MarkdownComponents{
			Parser:       extension.NewTableParser(),
			HTMLRenderer: extension.NewTableHTMLRenderer(),
		}
	case "strikethrough":
		return plugin.MarkdownComponents{
			Parser:       extension.NewStrikethroughParser(),
			HTMLRenderer: extension.NewStrikethroughHTMLRenderer(),
		}
	case "task-list":
		return plugin.MarkdownComponents{
			Parser:       extension.NewTaskListItemParser(),
			HTMLRenderer: extension.NewTaskListItemHTMLRenderer(),
		}
	case "definition-list":
		return plugin.MarkdownComponents{
			Parser:       extension.NewDefinitionListParser(),
			HTMLRenderer: extension.NewDefinitionListHTMLRenderer(),
		}
	case "footnote":
		return plugin.MarkdownComponents{
			Parser:       extension.NewFootnoteParser(),
			HTMLRenderer: extension.NewFootnoteHTMLRenderer(),
		}
	case "linkify":
		return plugin.MarkdownComponents{Parser: extension.NewLinkifyParser()}
	default:
		return plugin.MarkdownComponents{}
	}
}
