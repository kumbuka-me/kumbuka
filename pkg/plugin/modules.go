// Package plugin defines Kumbuka's in-process module contracts. Only trusted,
// compiled modules may implement native callbacks; installed code executes through
// the sandboxed runtime adapter. This package grants no persistence or I/O access.
package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kumbuka-me/sdk"
	"github.com/yuin/goldmark/v2/parser"
	goldhtml "github.com/yuin/goldmark/v2/renderer/html"
)

// Descriptor identifies a plugin independently of how it is distributed.
type Descriptor struct {
	// ID identifies the associated object.
	ID string
	// Name is the human-readable name.
	Name string
	// Description summarizes the associated object.
	Description string
	// DefaultEnabled indicates whether the plugin starts enabled by default.
	DefaultEnabled bool
	// Requires lists plugin IDs that must be active before this plugin.
	Requires []string
}

// SourceUsageRule is a cheap host-side selector for one source-aware module.
// Rules are hints only: modules without rules remain always active.
type SourceUsageRule struct {
	// Contains selects the module when the Markdown contains this literal text.
	Contains string
	// Fence selects fenced code by info-string language; "*" matches any fence.
	Fence string
	// Macro selects a standalone {{name ...}} invocation.
	Macro string
	// Substitution selects inline {{prefix:value}} references and extracts values.
	Substitution string
}

// SourceUsage describes how Kumbuka can identify one module without invoking plugin code.
type SourceUsage struct {
	// ModuleID identifies the module associated with source usage.
	ModuleID string
	// Rules contains the rules associated with source usage.
	Rules []SourceUsageRule
}

// SourceUsageProvider is implemented by runtime adapters that opt into cheap source selection.
// Implementations with no rules intentionally remain always active.
type SourceUsageProvider interface {
	SourceUsage() SourceUsage
}

// Context contains only render-local capabilities. RenderMarkdown returns
// intermediate HTML: the host must sanitize the complete document afterwards.
// Callbacks must not retain this context or mutate its maps.
type Context struct {
	// Context carries cancellation and request-scoped values.
	Context context.Context
	// Capabilities exposes request-scoped host capabilities.
	Capabilities map[string]Capability
	// Features contains request-scoped feature flags.
	Features map[string]bool
	// Macros contains request-scoped macro renderers.
	Macros map[string]MacroRenderer
	// ExportParameters contains request-local values keyed by plugin and field.
	ExportParameters map[string]map[string]map[string]string
	// RenderMarkdown renders nested Markdown through the host pipeline.
	RenderMarkdown func(string) (string, error)
}

// Replacement binds an opaque source token to Markdown inserted after all content preprocessors run.
type Replacement struct {
	// Token is the opaque source marker emitted by a content preprocessor.
	Token string
	// Value is the Markdown restored immediately before Goldmark parsing.
	Value string
	// Annotation identifies an optional inspector item that owns this replacement.
	Annotation string
}

// InspectorItem is one value exposed by a plugin-owned reading-page inspector.
type InspectorItem struct {
	// Key identifies the item within the inspector.
	Key string
	// Label is the human-readable item name.
	Label string
	// Value is the saved value shown by the inspector.
	Value string
	// Description is optional explanatory text.
	Description string
	// Occurrences is the number of replacements originating from this item.
	Occurrences int
	// Annotation identifies rendered spans associated with this item.
	Annotation string
}

// Inspector describes one plugin-owned reading-page inspector.
type Inspector struct {
	// ID identifies the inspector within its plugin.
	ID string
	// PluginID identifies the owning plugin.
	PluginID string
	// Name is the human-readable inspector title.
	Name string
	// Items contains values used by the current page.
	Items []InspectorItem
}

// ExportField describes one request-local plugin parameter exposed by page export UI.
type ExportField struct {
	// PluginID identifies the owning plugin.
	PluginID string
	// ModuleID identifies the owning content module.
	ModuleID string
	// Key identifies the exported resource item.
	Key string
	// Label is the human-readable field label.
	Label string
	// Value is the saved value used as the initial export value.
	Value string
	// Description is optional explanatory text.
	Description string
}

// PreparedContent contains Markdown after plugin-owned content preprocessing plus render metadata.
type PreparedContent struct {
	// Markdown is the transformed source passed to the Markdown renderer.
	Markdown string
	// Replacements restore opaque plugin substitutions immediately before parsing.
	Replacements []Replacement
	// Inspectors contains reading-page metadata contributed by enabled plugins.
	Inspectors []Inspector
	// ExportFields contains request-local export controls contributed by enabled plugins.
	ExportFields []ExportField
}

// ContentPreprocessor transforms page Markdown before the normal render pipeline.
// Its output is never recursively reprocessed by another pass; preprocessors run once
// in ascending priority order.
type ContentPreprocessor interface {
	// Priority orders content preprocessing. Lower values run first.
	Priority() int
	// PreprocessContent transforms source and may contribute render metadata.
	PreprocessContent(Context, string) (PreparedContent, error)
}

// AdminAction executes one explicit administrator-triggered plugin operation.
type AdminAction interface {
	// Run executes the action inside the authenticated administrator request context.
	Run(context.Context) error
}

// AdminActionModule describes one host-rendered administrator action owned by a plugin.
type AdminActionModule struct {
	// ID identifies the action within its plugin.
	ID string
	// Name is the human-readable button label.
	Name string
	// Description explains the action to administrators.
	Description string
	// Icon is the optional host icon shown for the action.
	Icon string
	// Action executes the sandboxed operation.
	Action AdminAction
}

// AdminResource describes one declarative plugin-owned record collection.
type AdminResource struct {
	// ID identifies the resource within its plugin.
	ID string
	// Name is the human-readable resource title.
	Name string
	// Description explains the resource to administrators.
	Description string
}

// EditorCompletion describes one resource-backed editor completion provider.
type EditorCompletion struct {
	// ID identifies the contribution within its plugin.
	ID string
	// Resource identifies the backing admin-resource module.
	Resource string
	// Trigger opens completion in the Markdown editor.
	Trigger string
	// Replacement formats one selected record into Markdown.
	Replacement string
	// LabelField and DetailField select resource fields shown in completion results.
	LabelField, DetailField string
}

// EditorInsert describes one declarative editor action owned by a plugin.
type EditorInsert struct {
	// ID identifies the contribution within its plugin.
	ID string
	// Name and Description are shown in editor insertion UI.
	Name, Description string
	// Markdown and Suffix describe inserted, wrapped, or line-prefixed source.
	Markdown, Suffix string
	// Placeholder supplies default selected text for wrap and prefix actions.
	Placeholder string
	// Mode and Group select generic editor behavior and toolbar placement.
	Mode, Group string
	// Icon is the optional host icon shown for the action.
	Icon string
	// Inline reports whether plain insertion should avoid surrounding line breaks.
	Inline bool
}

// Invocation is a serializable macro argument value carried by the WASM ABI.
type Invocation = json.RawMessage

// MacroRenderer binds request-local data to a registered macro.
type MacroRenderer func(Invocation) (string, error)

// ConditionalMacro optionally controls whether a macro can expand in a
// particular request. An unavailable invocation stays ordinary Markdown.
type ConditionalMacro interface {
	// Available reports whether the macro can expand in the current render context.
	Available(Context) bool
}

// ContextualMacro parses invocations using request-local render context.
type ContextualMacro interface {
	// ParseContext parses one invocation with access to request-local capabilities.
	ParseContext(Context, string) (Invocation, bool, error)
}

// Macro recognizes a standalone {{name ...}} invocation and produces untrusted HTML.
type Macro interface {
	// Name returns the registered contribution name.
	Name() string
	// Parse recognizes one macro invocation and returns serialized arguments.
	Parse(string) (Invocation, bool)
	// Render expands one parsed macro invocation into untrusted HTML.
	Render(Context, Invocation) (string, error)
}

// Preprocessor transforms Markdown before parsing, including nested blocks.
type Preprocessor interface {
	// Preprocess transforms Markdown before the core parser runs.
	Preprocess(Context, string) (string, error)
}

// MarkdownComponents contains the parser and HTML renderer portions contributed by one Markdown extension.
type MarkdownComponents struct {
	// Parser extends Goldmark parsing for the current render when non-nil.
	Parser parser.Extension
	// HTMLRenderer extends Goldmark HTML rendering for the current render when non-nil.
	HTMLRenderer goldhtml.Extension
}

// MarkdownExtension creates fresh Goldmark components for each conversion.
// This is a host-side adapter, not an API for loading native community code.
// Extensions can contribute parsers, AST transformers, and HTML node renderers.
type MarkdownExtension interface {
	// Components returns fresh Goldmark parser and renderer extensions for the current render.
	Components(Context) MarkdownComponents
}

// CodeHighlightResult contains one highlighter response. HTML remains untrusted
// until Kumbuka sanitizes the complete rendered document.
type CodeHighlightResult struct {
	// HTML contains highlighted block markup when Matched is true.
	HTML string
	// Matched reports whether the provider recognized the fenced-code language.
	Matched bool
}

// CodeHighlighter highlights one fenced code block. Only one provider may be
// active at a time so fenced-code ownership is deterministic.
type CodeHighlighter interface {
	// Highlight renders source for language or reports that the language is unsupported.
	Highlight(Context, string, string) (CodeHighlightResult, error)
}

// CodeHighlighterModule describes one exclusive code-highlighting contribution.
type CodeHighlighterModule struct {
	// ID identifies the module within its plugin.
	ID string
	// CSS is an optional package stylesheet filtered and scoped by Kumbuka.
	CSS string
	// Highlighter performs the sandboxed highlighting operation.
	Highlighter CodeHighlighter
}

// Postprocessor transforms the complete HTML, including macro output, before
// the trusted central sanitizer. It runs once per document, not per nested block.
type Postprocessor interface {
	// Postprocess transforms rendered HTML before central sanitization.
	Postprocess(Context, string) (string, error)
}

// WidgetRequest describes one host surface and optional current page.
type WidgetRequest struct {
	// Surface identifies the host placement being rendered.
	Surface string
	// Page contains authorized current-page metadata for page-scoped widgets.
	Page *sdk.Page
}

// WidgetResult contains untrusted widget HTML and safe host-rendered actions.
type WidgetResult struct {
	// HTML is sanitized by Kumbuka before it reaches the page template.
	HTML string
	// Actions are rendered by the host rather than emitted as trusted plugin HTML.
	Actions []sdk.WidgetAction
}

// Widget renders one optional host-surface contribution.
type Widget interface {
	// Render returns untrusted widget HTML and optional host actions.
	Render(Context, WidgetRequest) (WidgetResult, error)
}

// WidgetCommandRequest describes one host-mediated command selected from a widget.
type WidgetCommandRequest struct {
	// Surface identifies the host placement that emitted the command.
	Surface string
	// Page contains authorized current-page metadata when available.
	Page *sdk.Page
	// Action identifies the command selected by the user.
	Action string
}

// WidgetCommander handles state-changing widget commands through the host boundary.
type WidgetCommander interface {
	// Command executes one validated widget command and returns safe navigation metadata.
	Command(Context, WidgetCommandRequest) (sdk.WidgetCommandResult, error)
}

// WidgetModule describes one widget contribution and its host surface.
type WidgetModule struct {
	// ID identifies the module within its plugin.
	ID string
	// Surface selects the host placement.
	Surface string
	// Width is an optional host layout hint.
	Width string
	// Order controls deterministic placement within the surface.
	Order int
	// Widget performs sandboxed widget rendering.
	Widget Widget
}

// ExportRequest contains the authorized current page and stored Markdown source.
type ExportRequest struct {
	// Page contains public metadata for the page being exported.
	Page sdk.Page
	// Source contains the stored Markdown source for the page.
	Source string
}

// Exporter produces one bounded downloadable file for the current page.
type Exporter interface {
	// Export returns the complete file produced for one authorized page.
	Export(Context, ExportRequest) (sdk.ExportFile, error)
}

// ExporterModule describes one executable page export contribution.
type ExporterModule struct {
	// ID identifies the exporter within its plugin.
	ID string
	// Name and Description are shown in host export UI.
	Name, Description string
	// Icon is the optional host icon shown for the exporter.
	Icon string
	// Order controls deterministic placement among plugin exporters.
	Order int
	// Exporter performs the sandboxed export operation.
	Exporter Exporter
}

// BrowserModule declares browser assets contributed by one plugin module.
type BrowserModule struct {
	// ID, JavaScript, and CSS identify the module and its optional asset paths.
	ID, JavaScript, CSS string
}

// EditorExtension reserves editor contribution metadata without loading assets.
type EditorExtension struct {
	// ID and BrowserModuleID identify the editor extension and its browser module.
	ID, BrowserModuleID string
}

// SettingsModule describes a settings contribution without exposing core storage.
type SettingsModule struct {
	// ID and Name identify the settings contribution and its display name.
	ID, Name string
	// Requires lists prerequisite feature keys within the owning package.
	Requires []string
}

// ContentStyle declares a stylesheet that core may expose to rendered page
// content after applying its parent-document CSS safety filter.
type ContentStyle struct {
	// ID identifies the contribution within its plugin.
	ID string
	// CSS is the validated package-relative stylesheet asset path.
	CSS string
}

// RenderPolicy declares one semantic rendering marker shared by active plugins.
type RenderPolicy struct {
	// ID identifies the contribution within its plugin.
	ID string
	// Policy is an opaque public policy identifier; core does not interpret it.
	Policy string
}

// Contributions is registered and removed atomically under its owner's ID.
// Order within a stage is registration order, then slice order.
type Contributions struct {
	// ContentPreprocessors transform application Markdown before the normal render pipeline.
	ContentPreprocessors []ContentPreprocessor
	// Preprocessors run before core Markdown parsing in contribution order.
	Preprocessors []Preprocessor
	// MarkdownExtensions contribute fresh Goldmark extensions per render.
	MarkdownExtensions []MarkdownExtension
	// CodeHighlighters contains an exclusive fenced-code highlighting provider.
	CodeHighlighters []CodeHighlighterModule
	// Postprocessors run on rendered HTML before central sanitization.
	Postprocessors []Postprocessor
	// Macros contains request-scoped macro renderers.
	Macros []Macro
	// Widgets declares optional UI contributions rendered on host surfaces.
	Widgets []WidgetModule
	// Exporters declares optional page download formats implemented by plugins.
	Exporters []ExporterModule
	// BrowserModules declares browser assets exposed for enabled plugins.
	BrowserModules []BrowserModule
	// EditorExtensions declares editor integrations owned by the plugin.
	EditorExtensions []EditorExtension
	// AdminActions declares administrator-triggered operations rendered by Kumbuka.
	AdminActions []AdminActionModule
	// AdminResources declares plugin-owned record collections rendered by Kumbuka.
	AdminResources []AdminResource
	// EditorCompletions declares resource-backed editor completion providers.
	EditorCompletions []EditorCompletion
	// EditorInserts declares plugin-owned editor actions.
	EditorInserts []EditorInsert
	// SettingsModules declares settings integrations owned by the plugin.
	SettingsModules []SettingsModule
	// ContentStyles declares safe parent-document styles owned by the plugin.
	ContentStyles []ContentStyle
	// RenderPolicies declares semantic rendering markers owned by the plugin.
	RenderPolicies []RenderPolicy
}

// BindMacro adapts a typed, request-scoped renderer to serialized arguments.
func BindMacro[T any](render func(T) (string, error)) MacroRenderer {
	if render == nil {
		return nil
	}
	return func(value Invocation) (string, error) {
		var options T
		if err := json.Unmarshal(value, &options); err != nil {
			return "", err
		}
		return render(options)
	}
}

// BoundMacro adapts an existing parser to a request-local renderer. When no
// binding exists it leaves the invocation literal, unless EmptyWhenUnbound is
// set to preserve a module's established empty-context behavior.
type BoundMacro[T any] struct {
	// MacroName is the registered macro identifier.
	MacroName string
	// ParseOptions recognizes source syntax and returns typed macro options.
	ParseOptions func(string) (T, bool)
	// EmptyWhenUnbound renders an empty result when no request-local binding exists.
	EmptyWhenUnbound bool
}

// Name returns the registered contribution name.
func (m BoundMacro[T]) Name() string { return m.MacroName }

// Available reports whether the contribution is available in the current context.
func (m BoundMacro[T]) Available(ctx Context) bool {
	return m.EmptyWhenUnbound || ctx.Macros[m.Name()] != nil
}

// Parse recognizes one macro invocation and returns serialized arguments.
func (m BoundMacro[T]) Parse(line string) (Invocation, bool) {
	options, ok := m.ParseOptions(line)
	if !ok {
		return nil, false
	}
	encoded, err := json.Marshal(options)
	return encoded, err == nil
}

// Render expands one parsed macro invocation into untrusted HTML.
func (m BoundMacro[T]) Render(ctx Context, value Invocation) (string, error) {
	if render := ctx.Macros[m.Name()]; render != nil {
		return render(value)
	}
	if m.EmptyWhenUnbound {
		return "", nil
	}
	return "", fmt.Errorf("macro %s has no request binding", m.Name())
}

// Guard converts synchronous native module panics into render errors. Native
// modules remain trusted; installed contributions execute in the WASM sandbox.
func Guard[T any](owner string, run func() (T, error)) (result T, err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("plugin %s panicked", owner)
		}
	}()
	result, err = run()
	if err != nil {
		err = fmt.Errorf("plugin %s: %w", owner, err)
	}
	return result, err
}
